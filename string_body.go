package falcore

import (
	"io"
	//"io/ioutil"
	"net/http"
	"strings"
	"bytes"
	"time"
)

// Keeps the body of a request in a string so it can be re-read at each stage of the pipeline
// implements io.ReadCloser to match http.Request.Body

type StringBody struct {
	BodyBuffer *bytes.Reader
	bpe *bufferPoolEntry
}

type StringBodyFilter struct {
	pool *bufferPool
}

func NewStringBodyFilter() *StringBodyFilter {
	sbf := &StringBodyFilter{}
	sbf.pool = newBufferPool(100, 1024)
	return sbf
}
func (sbf *StringBodyFilter) FilterRequest(request *Request) *http.Response {
	req := request.HttpRequest
	// This caches the request body so that multiple filters can iterate it
	if req.Method == "POST" || req.Method == "PUT" {
		sb, err := sbf.readRequestBody(req)
		if sb == nil || err != nil {
			request.CurrentStage.Status = 3 // Skip
			Debug("%s No Req Body or Ignored: %v", request.ID, err)
		}
	} else {
		request.CurrentStage.Status = 1 // Skip
	}
	return nil
}

// reads the request body and replaces the buffer with self
// returns nil if the body is multipart and not replaced
func (sbf *StringBodyFilter)readRequestBody(r *http.Request) (sb *StringBody, err error) {
	start := time.Now()
	ct := r.Header.Get("Content-Type")
	// leave it on the buffer if we're multipart
	if strings.SplitN(ct, ";", 2)[0] != "multipart/form-data" && r.ContentLength > 0 {
		sb = &StringBody{}
		const maxFormSize = int64(10 << 20) // 10 MB is a lot of text.
		//rr := io.LimitReader(r.Body, maxFormSize+1)
		rr := r.Body
		sb.bpe = sbf.pool.take(rr)
		cumu := time.Since(start)
		Warn("C1: %v", cumu)
		b, e := sb.bpe.br.ReadBytes(0)
		cumu = time.Since(start)
		Warn("C2: %v", cumu)
		//Error("B: %v, E: %v\n", b, e)
		if e != nil && e != io.EOF {
			return nil, e
		}
		sb.BodyBuffer = bytes.NewReader(b)
		cumu = time.Since(start)
		Warn("C3: %v", cumu)
		go r.Body.Close()
		r.Body = sb
		cumu = time.Since(start)
		Warn("C4: %v", cumu)
		return sb, nil
	}
	return nil, nil // ignore	
}

// Returns a buffer used in the FilterRequest stage to a buffer pool
// this speeds up this filter significantly by reusing buffers
func (sbf *StringBodyFilter)ReturnBuffer(request *Request) {
	if sb, ok := request.HttpRequest.Body.(*StringBody); ok {
		sbf.pool.give(sb.bpe)	
	}
}

// Insert this in the response pipeline to return the buffer pool for the request body
// If there is an appropriate place in your flow, you can call ReturnBuffer explicitly
func (sbf *StringBodyFilter) FilterResponse(request *Request, res *http.Response) {
	sbf.ReturnBuffer(request)
}


func (sb *StringBody) Read(b []byte) (n int, err error) {
	return sb.BodyBuffer.Read(b)
}

func (sb *StringBody) Close() error {
	// start over
	sb.BodyBuffer.Seek(0, 0)
	return nil
}
