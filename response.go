package falcore

import (
	"net/http"
	"strings"
)

func SimpleResponse(req *http.Request, status int, headers http.Header, body string) *http.Response {
	res := new(http.Response)
	body_rdr := (*fixedResBody)(strings.NewReader(body))
	res.StatusCode = status
	res.ProtoMajor = 1
	res.ProtoMinor = 1
	res.ContentLength = int64((*strings.Reader)(body_rdr).Len())
	res.Request = req
	res.Header = make(map[string][]string)
	res.Body = body_rdr
	if headers != nil {
		res.Header = headers
	}
	return res
}

// string type for response objects

type fixedResBody strings.Reader

func (s *fixedResBody) Close() error {
	return nil
}

func (s *fixedResBody) Read(b []byte) (int, error) {
	return (*strings.Reader)(s).Read(b)
}

func RedirectResponse(req *http.Request, url string) *http.Response {
	res := new(http.Response)
	res.StatusCode = 302
	res.ProtoMajor = 1
	res.ProtoMinor = 1
	res.ContentLength = 0
	res.Request = req
	res.Header = make(map[string][]string)
	res.Header.Set("Location", url)
	return res
}
