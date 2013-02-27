package falcore

import (
	"net/http"
)

// Filter incomming requests and optionally return a response or nil.
// Filters are chained together into a flow (the Pipeline) which will terminate
// if the Filter returns a response.
type RequestFilter interface {
	FilterRequest(req *Request) *http.Response
}

// Helper to create a Filter by just passing in a func
//    filter = NewRequestFilter(func(req *Request) *http.Response {
//			req.Headers.Add("X-Falcore", "is_cool")
//			return
//		})
func NewRequestFilter(f func(req *Request) *http.Response) RequestFilter {
	rf := new(genericRequestFilter)
	rf.f = f
	return rf
}

type genericRequestFilter struct {
	f func(req *Request) *http.Response
}

func (f *genericRequestFilter) FilterRequest(req *Request) *http.Response {
	return f.f(req)
}

// Filter outgoing responses. This can be used to modify the response
// before it is sent.  Modifying the request at this point will have no
// effect.
type ResponseFilter interface {
	FilterResponse(req *Request, res *http.Response)
}

// Helper to create a Filter by just passing in a func
//    filter = NewResponseFilter(func(req *Request, res *http.Response) {
//			// some crazy response magic
//			return
//		})
func NewResponseFilter(f func(req *Request, res *http.Response)) ResponseFilter {
	rf := new(genericResponseFilter)
	rf.f = f
	return rf
}

type genericResponseFilter struct {
	f func(req *Request, res *http.Response)
}

func (f *genericResponseFilter) FilterResponse(req *Request, res *http.Response) {
	f.f(req, res)
}
