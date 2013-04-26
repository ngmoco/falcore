package falcore

import (
	"io"
)

type continueReader struct {
	req    *Request
	r      io.ReadCloser
	opened bool
}

var _ io.ReadCloser = new(continueReader)

func (r *continueReader) Read(p []byte) (int, error) {
	// sent 100 continue the first time we try to read the body
	if !r.opened {
		resp := SimpleResponse(r.req.HttpRequest, 100, nil, "")
		if err := resp.Write(r.req.Connection); err != nil {
			return 0, err
		}
		r.req = nil
		r.opened = true
	}
	return r.r.Read(p)
}

func (r *continueReader) Close() error {
	r.req = nil
	return r.r.Close()
}
