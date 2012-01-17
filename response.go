package falcore

import (
	"http"
	"strings"
	"os"
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

func (s *fixedResBody) Close() os.Error {
	return nil
}

func (s *fixedResBody) Read(b []byte) (int, os.Error) {
	return (*strings.Reader)(s).Read(b)
}
