package falcore

import (
	"strings"
	"http"
	"io/ioutil"
	"os"
	"io"
)

// Keeps the body of a request in a string so it can be re-read at each stage of the pipeline
// implements io.ReadCloser to match http.Request.Body

type StringBody struct {
	BodyString string
	BodyBuffer *strings.Reader
}

type StringBodyFilter struct{}

func (sbf *StringBodyFilter) FilterRequest(request *Request) *http.Response {
	req := request.HttpRequest
	// This caches the request body so that multiple filters can iterate it
	if req.Method == "POST" || req.Method == "PUT" {
		sb, err := ReadRequestBody(req)
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
func ReadRequestBody(r *http.Request) (sb *StringBody, err os.Error) {
	ct := r.Header.Get("Content-Type")
	// leave it on the buffer if we're multipart
	if strings.SplitN(ct, ";", 2)[0] != "multipart/form-data" && r.ContentLength > 0 {
		sb = new(StringBody)
		const maxFormSize = int64(10 << 20) // 10 MB is a lot of text.
		b, e := ioutil.ReadAll(io.LimitReader(r.Body, maxFormSize+1))
		if e != nil {
			return nil, e
		}
		sb.BodyString = string(b)
		sb.Close() // to create our buffer
		r.Body.Close()
		r.Body = sb
		return sb, nil
	}
	return nil, nil // ignore	
}

func (sb *StringBody) Read(b []byte) (n int, err os.Error) {
	return sb.BodyBuffer.Read(b)
}

func (sb *StringBody) Close() os.Error {
	// start over
	sb.BodyBuffer = strings.NewReader(sb.BodyString)
	return nil
}
