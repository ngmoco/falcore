package falcore

import (
	"container/list"
	"fmt"
	"hash"
	"hash/crc32"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"time"
)

// Request wrapper
// 
// The request is wrapped so that useful information can be kept 
// with the request as it moves through the pipeline.  
//
// A pointer is kept to the originating Connection.
//
// There is a unique ID assigned to each request.  This ID is not 
// globally unique to keep it shorter for logging purposes.  It is 
// possible to have duplicates though very unlikely over the period 
// of a day or so.  It is a good idea to log the ID in any custom
// log statements so that individual requests can easily be grepped
// from busy log files.
//
// Falcore collects performance statistics on every stage of the 
// pipeline.  The stats for the request are kept in PipelineStageStats.
// This structure will only be complete in the Request passed to the
// pipeline RequestDoneCallback.  Overhead will only be available in
// the RequestDoneCallback and it's the difference between the total
// request time and the sums of the stage times.  It will include things
// like pipeline iteration and the stat collection itself.
// 
// See falcore.PipelineStageStat docs for more info.
// 
// The Signature is also a cool feature. See the 
type Request struct {
	ID                 string
	StartTime          time.Time
	EndTime            time.Time
	HttpRequest        *http.Request
	Connection         net.Conn
	RemoteAddr         *net.TCPAddr
	PipelineStageStats *list.List
	CurrentStage       *PipelineStageStat
	pipelineHash       hash.Hash32
	piplineTot         time.Duration
	Overhead           time.Duration
	Context            map[string]interface{}
}

// Used internally to create and initialize a new request.
func newRequest(request *http.Request, conn net.Conn, startTime time.Time) *Request {
	fReq := new(Request)
	fReq.Context = make(map[string]interface{})
	fReq.HttpRequest = request
	fReq.StartTime = startTime
	fReq.Connection = conn
	if conn != nil {
		fReq.RemoteAddr = conn.RemoteAddr().(*net.TCPAddr)
	}
	// create a semi-unique id to track a connection in the logs
	// ID is the least significant decimal digits of time with some randomization
	// the last 3 zeros of time.Nanoseconds appear to always be zero		
	var ut = fReq.StartTime.UnixNano()
	fReq.ID = fmt.Sprintf("%010x", (ut-(ut-(ut%1e12)))+int64(rand.Intn(999)))
	fReq.PipelineStageStats = list.New()
	fReq.pipelineHash = crc32.NewIEEE()
	return fReq
}

// Returns a completed falcore.Request and response after running the single filter stage
// The PipelineStageStats is completed in the returned Request 
// The falcore.Request.Connection and falcore.Request.RemoteAddr are nil
func TestWithRequest(request *http.Request, filter RequestFilter) (*Request, *http.Response) {
	r := newRequest(request, nil, time.Now())
	t := reflect.TypeOf(filter)
	r.startPipelineStage(t.String())
	res := filter.FilterRequest(r)
	r.finishPipelineStage()
	r.finishRequest()
	return r, res
}

// Starts a new pipeline stage and makes it the CurrentStage.
func (fReq *Request) startPipelineStage(name string) {
	fReq.CurrentStage = NewPiplineStage(name)
	fReq.PipelineStageStats.PushBack(fReq.CurrentStage)
}

// Finishes the CurrentStage.
func (fReq *Request) finishPipelineStage() {
	fReq.CurrentStage.EndTime = time.Now()
	fReq.finishCommon()
}

// Appends an already completed PipelineStageStat directly to the list
func (fReq *Request) appendPipelineStage(pss *PipelineStageStat) {
	fReq.PipelineStageStats.PushBack(pss)
	fReq.CurrentStage = pss
	fReq.finishCommon()
}

// Does some required bookeeping for the pipeline and the pipeline signature
func (fReq *Request) finishCommon() {
	fReq.pipelineHash.Write([]byte(fReq.CurrentStage.Name))
	fReq.pipelineHash.Write([]byte{fReq.CurrentStage.Status})
	fReq.piplineTot += fReq.CurrentStage.EndTime.Sub(fReq.CurrentStage.StartTime)
}

// The Signature will only be complete in the RequestDoneCallback.  At
// any given time, the Signature is a crc32 sum of all the finished
// pipeline stages combining PipelineStageStat.Name and PipelineStageStat.Status.
// This gives a unique signature for each unique path through the pipeline.
// To modify the signature for your own use, just set the 
// request.CurrentStage.Status in your RequestFilter or ResponseFilter.
func (fReq *Request) Signature() string {
	return fmt.Sprintf("%X", fReq.pipelineHash.Sum32())
}

// Call from RequestDoneCallback.  Logs a bunch of information about the 
// request to the falcore logger. This is a pretty big hit to performance 
// so it should only be used for debugging or development.  The source is a 
// good example of how to get useful information out of the Request. 
func (fReq *Request) Trace() {
	reqTime := TimeDiff(fReq.StartTime, fReq.EndTime)
	req := fReq.HttpRequest
	Trace("%s [%s] %s%s Sig=%s Tot=%.4f", fReq.ID, req.Method, req.Host, req.URL, fReq.Signature(), reqTime)
	l := fReq.PipelineStageStats
	for e := l.Front(); e != nil; e = e.Next() {
		pss, _ := e.Value.(*PipelineStageStat)
		dur := TimeDiff(pss.StartTime, pss.EndTime)
		Trace("%s %-30s S=%d Tot=%.4f %%=%.2f", fReq.ID, pss.Name, pss.Status, dur, dur/reqTime*100.0)
	}
	Trace("%s %-30s S=0 Tot=%.4f %%=%.2f", fReq.ID, "Overhead", float32(fReq.Overhead)/1.0e9, float32(fReq.Overhead)/1.0e9/reqTime*100.0)
}

func (fReq *Request) finishRequest() {
	fReq.EndTime = time.Now()
	fReq.Overhead = fReq.EndTime.Sub(fReq.StartTime) - fReq.piplineTot
}

// Container for keeping stats per pipeline stage 
// Name for filter stages is reflect.TypeOf(filter).String()[1:] and the Status is 0 unless
// it is changed explicitly in the Filter or Router.
// 
// For the Status, the falcore library will not apply any specific meaning to the status 
// codes but the following are suggested conventional usages that we have found useful
//
//   type PipelineStatus byte
//   const (
// 	    Success PipelineStatus = iota	// General Run successfully
//	    Skip								// Skipped (all or most of the work of this stage)
//	    Fail								// General Fail
//	    // All others may be used as custom status codes
//   )
type PipelineStageStat struct {
	Name      string
	Status    byte
	StartTime time.Time
	EndTime   time.Time
}

func NewPiplineStage(name string) *PipelineStageStat {
	pss := new(PipelineStageStat)
	pss.Name = name
	pss.StartTime = time.Now()
	return pss
}
