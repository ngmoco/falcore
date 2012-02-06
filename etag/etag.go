package etag

import (
	"github.com/ngmoco/falcore"
	"net/http"
)

// falcore/etag.Filter is a falcore.ResponseFilter that matches
// the response's Etag header against the request's If-None-Match
// header.  If they match, the filter will return a '304 Not Modifed'
// status and no body.
// 
// Ideally, Etag filtering is performed as soon as possible as
// you may be able to skip generating the response body at all.
// Even as a last step, you will see a significant benefit if
// clients are well behaved.
// 
type Filter struct {
}

func (f *Filter) FilterResponse(request *falcore.Request, res *http.Response) {
	request.CurrentStage.Status = 1 // Skipped (default)
	if if_none_match := request.HttpRequest.Header.Get("If-None-Match"); if_none_match != "" {
		if res.StatusCode == 200 && res.Header.Get("Etag") == if_none_match {
			res.StatusCode = 304
			res.Status = "304 Not Modified"
			res.Body.Close()
			res.Body = nil
			res.ContentLength = 0
			request.CurrentStage.Status = 0 // Success
		}
	}
}
