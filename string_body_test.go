package falcore

import (
	"testing"
	"http"
	"time"
	"bytes"
)

func TestStringBody(t *testing.T) {
	expected := []byte("HOT HOT HOT!!!")
	tmp, _ := http.NewRequest("POST", "/hello", bytes.NewBuffer(expected))
	tmp.Header.Set("Content-Type", "text/plain")
	tmp.ContentLength = int64(len(expected))
	req := newRequest(tmp, nil, time.Nanoseconds())
	req.startPipelineStage("StringBodyTest")

	sbf := &StringBodyFilter{}
	sbf.FilterRequest(req)

	if sb, ok := req.HttpRequest.Body.(*StringBody); ok {
		if bytes.Compare([]byte(sb.BodyString), expected) != 0 {
			t.Errorf("Body string not read %q expected %q", sb.BodyString, expected)
		}
	} else {
		t.Errorf("Body not replaced with StringBody")
	}

	if req.CurrentStage.Status != 0 {
		t.Errorf("SBF failed to parse POST with status %d", req.CurrentStage.Status)
	}

	var body []byte = make([]byte, 100)
	l, _ := req.HttpRequest.Body.Read(body)
	if bytes.Compare(body[0:l], expected) != 0 {
		t.Errorf("Failed to read the right bytes %q expected %q", body, expected)

	}

	l, _ = req.HttpRequest.Body.Read(body)
	if l != 0 {
		t.Errorf("Should have read zero!")
	}

	// Close resets the buffer
	req.HttpRequest.Body.Close()

	l, _ = req.HttpRequest.Body.Read(body)
	if bytes.Compare(body[0:l], expected) != 0 {
		t.Errorf("Failed to read the right bytes after calling Close %q expected %q", body, expected)

	}

}
