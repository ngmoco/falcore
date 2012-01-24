package falcore

import (
	"bytes"
	"container/list"
	"net/http"
	"testing"
	"time"
)

func validGetRequest() *Request {
	tmp, _ := http.NewRequest("GET", "/hello", bytes.NewBuffer(make([]byte, 0)))
	return newRequest(tmp, nil, time.Now())
}

var stageTrack *list.List

func doStageTrack() {
	i := 0
	if stageTrack.Len() > 0 {
		i = stageTrack.Back().Value.(int)
	}
	stageTrack.PushBack(i + 1)
}

func sumFilter(req *Request) *http.Response {
	doStageTrack()
	return nil
}

func sumResponseFilter(*Request, *http.Response) {
	doStageTrack()
}

func successFilter(req *Request) *http.Response {
	doStageTrack()
	return SimpleResponse(req.HttpRequest, 200, nil, "OK")
}

func TestPipelineNoResponse(t *testing.T) {
	p := NewPipeline()

	stageTrack = list.New()
	f := NewRequestFilter(sumFilter)

	p.Upstream.PushBack(f)
	p.Upstream.PushBack(f)
	p.Upstream.PushBack(f)

	//response := new(http.Response)
	response := p.execute(validGetRequest())

	if stageTrack.Len() != 3 {
		t.Fatalf("Wrong number of stages executed: %v expected %v", stageTrack.Len(), 3)
	}
	if sum, ok := stageTrack.Back().Value.(int); ok {
		if sum != 3 {
			t.Errorf("Pipeline stages did not complete %v expected %v", sum, 3)
		}
	}
	if response.StatusCode != 404 {
		t.Errorf("Pipeline response code wrong: %v expected %v", response.StatusCode, 404)
	}
}

func TestPipelineOKResponse(t *testing.T) {
	p := NewPipeline()

	stageTrack = list.New()
	f := NewRequestFilter(sumFilter)

	p.Upstream.PushBack(f)
	p.Upstream.PushBack(f)
	p.Upstream.PushBack(NewRequestFilter(successFilter))
	p.Upstream.PushBack(f)

	response := p.execute(validGetRequest())

	if stageTrack.Len() != 3 {
		t.Fatalf("Wrong number of stages executed: %v expected %v", stageTrack.Len(), 3)
	}
	if sum, ok := stageTrack.Back().Value.(int); ok {
		if sum != 3 {
			t.Errorf("Pipeline stages did not complete %v expected %v", sum, 3)
		}
	}
	if response.StatusCode != 200 {
		t.Errorf("Pipeline response code wrong: %v expected %v", response.StatusCode, 200)
	}
}

func TestPipelineResponseFilter(t *testing.T) {
	p := NewPipeline()

	stageTrack = list.New()
	f := NewRequestFilter(sumFilter)

	p.Upstream.PushBack(f)
	p.Upstream.PushBack(NewRequestFilter(successFilter))
	p.Upstream.PushBack(f)
	p.Downstream.PushBack(NewResponseFilter(sumResponseFilter))
	p.Downstream.PushBack(NewResponseFilter(sumResponseFilter))

	//response := new(http.Response)
	req := validGetRequest()
	response := p.execute(req)

	stages := 4
	// check basic execution
	if stageTrack.Len() != stages {
		t.Fatalf("Wrong number of stages executed: %v expected %v", stageTrack.Len(), stages)
	}
	if sum, ok := stageTrack.Back().Value.(int); ok {
		if sum != stages {
			t.Errorf("Pipeline stages did not complete %v expected %v", sum, stages)
		}
	}
	// check status
	if response.StatusCode != 200 {
		t.Errorf("Pipeline response code wrong: %v expected %v", response.StatusCode, 200)
	}
	req.finishRequest()
	if req.Signature() != "F7F5165F" {
		t.Errorf("Signature failed: %v expected %v", req.Signature(), "F7F5165F")
	}
	if req.PipelineStageStats.Len() != stages {
		t.Errorf("PipelineStageStats incomplete: %v expected %v", req.PipelineStageStats.Len(), stages)
	}
	//req.Trace()

}

func TestPipelineStatsChecksum(t *testing.T) {
	p := NewPipeline()

	stageTrack = list.New()
	f := NewRequestFilter(sumFilter)

	p.Upstream.PushBack(f)
	p.Upstream.PushBack(NewRequestFilter(func(req *Request) *http.Response {
		doStageTrack()
		req.CurrentStage.Status = 1
		return nil
	}))
	p.Upstream.PushBack(NewRequestFilter(successFilter))
	p.Downstream.PushBack(NewResponseFilter(sumResponseFilter))
	p.Downstream.PushBack(NewResponseFilter(sumResponseFilter))

	//response := new(http.Response)
	req := validGetRequest()
	response := p.execute(req)

	stages := 5
	// check basic execution
	if stageTrack.Len() != stages {
		t.Fatalf("Wrong number of stages executed: %v expected %v", stageTrack.Len(), stages)
	}
	if sum, ok := stageTrack.Back().Value.(int); ok {
		if sum != stages {
			t.Errorf("Pipeline stages did not complete %v expected %v", sum, stages)
		}
	}
	// check status
	if response.StatusCode != 200 {
		t.Errorf("Pipeline response code wrong: %v expected %v", response.StatusCode, 200)
	}
	req.finishRequest()
	if req.Signature() != "CA843113" {
		t.Errorf("Signature failed: %v expected %v", req.Signature(), "CA843113")
	}
	if req.PipelineStageStats.Len() != stages {
		t.Errorf("PipelineStageStats incomplete: %v expected %v", req.PipelineStageStats.Len(), stages)
	}
	//req.Trace()

}
