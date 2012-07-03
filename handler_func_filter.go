package falcore

import (
	"net/http"
	"io"
	"fmt"
)

// Implements the RequestFilter using a http.HandleFunc to produce the response
// This will always return a response due to the requirements of the http.HandleFunc
// interface.
type HandlerFuncFilter struct {
	handler http.HandlerFunc
}

func NewHandlerFuncFilter(handler http.HandlerFunc) (*HandlerFuncFilter) {
	return &HandlerFuncFilter{ handler: handler}
}

func (h *HandlerFuncFilter) FilterRequest(req *Request) *http.Response {
	rw, respc := newPopulateResponseWriter()
	// this must be done concurrently so that the HandlerFunc can write the response
	// while falcore is copying it to the socket
	go func() {
		h.handler(rw, req.HttpRequest)
		rw.finish()
	}()
	return <-respc
}

// copied from net/http/filetransport.go
func newPopulateResponseWriter() (*populateResponse, <-chan *http.Response) {
	pr, pw := io.Pipe()
	rw := &populateResponse{
		ch: make(chan *http.Response),
		pw: pw,
		res: &http.Response{
			Proto:      "HTTP/1.0",
			ProtoMajor: 1,
			Header:     make(http.Header),
			Close:      true,
			Body:       pr,
		},
	}
	return rw, rw.ch
}

// populateResponse is a ResponseWriter that populates the *Response
// in res, and writes its body to a pipe connected to the response
// body. Once writes begin or finish() is called, the response is sent
// on ch.
type populateResponse struct {
	res          *http.Response
	ch           chan *http.Response
	wroteHeader  bool
	hasContent   bool
	sentResponse bool
	pw           *io.PipeWriter
}

func (pr *populateResponse) finish() {
	if !pr.wroteHeader {
		pr.WriteHeader(500)
	}
	if !pr.sentResponse {
		pr.sendResponse()
	}
	pr.pw.Close()
}

func (pr *populateResponse) sendResponse() {
	if pr.sentResponse {
		return
	}
	pr.sentResponse = true

	if pr.hasContent {
		pr.res.ContentLength = -1
	}
	pr.ch <- pr.res
}

func (pr *populateResponse) Header() http.Header {
	return pr.res.Header
}

func (pr *populateResponse) WriteHeader(code int) {
	if pr.wroteHeader {
		return
	}
	pr.wroteHeader = true

	pr.res.StatusCode = code
	pr.res.Status = fmt.Sprintf("%d %s", code, http.StatusText(code))
}

func (pr *populateResponse) Write(p []byte) (n int, err error) {
	if !pr.wroteHeader {
		pr.WriteHeader(http.StatusOK)
	}
	pr.hasContent = true
	if !pr.sentResponse {
		pr.sendResponse()
	}
	return pr.pw.Write(p)
}

