package compression

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"github.com/ngmoco/falcore"
	"io"
	"net/http"
	"strings"
)

var DefaultTypes = []string{"text/plain", "text/html", "application/json", "text/xml"}

type Filter struct {
	types []string
}

func NewFilter(types []string) *Filter {
	f := new(Filter)
	if types != nil {
		f.types = types
	} else {
		f.types = DefaultTypes
	}
	return f
}

func (c *Filter) FilterResponse(request *falcore.Request, res *http.Response) {
	req := request.HttpRequest
	if accept := req.Header.Get("Accept-Encoding"); accept != "" {

		// Is content an acceptable type for encoding?
		var compress = false
		var content_type = res.Header.Get("Content-Type")
		for _, t := range c.types {
			if content_type == t {
				compress = true
				break
			}
		}

		// Is the content already compressed
		if res.Header.Get("Content-Encoding") != "" {
			compress = false
		}

		if !compress {
			request.CurrentStage.Status = 1 // Skip
			return
		}

		// Figure out which encoding to use
		options := strings.Split(accept, ",")
		var mode string
		for _, opt := range options {
			if m := strings.TrimSpace(opt); m == "gzip" || m == "deflate" {
				mode = m
				break
			}
		}

		var compressor io.WriteCloser
		var buf = bytes.NewBuffer(make([]byte, 0, 1024))
		switch mode {
		case "gzip":
			compressor = gzip.NewWriter(buf)
		case "deflate":
			comp, err := flate.NewWriter(buf, -1)
			if err != nil {
				falcore.Error("Compression Error: %v", err)
				request.CurrentStage.Status = 1 // Skip
				return
			}
			compressor = comp
		default:
			request.CurrentStage.Status = 1 // Skip
			return
		}

		// Perform compression
		r := make([]byte, 1024)
		var err error
		var i int
		for err == nil {
			i, err = res.Body.Read(r)
			compressor.Write(r[0:i])
		}
		compressor.Close()
		res.Body.Close()

		res.ContentLength = int64(buf.Len())
		res.Body = (*filteredBody)(buf)
		res.Header.Set("Content-Encoding", mode)
	} else {
		request.CurrentStage.Status = 1 // Skip
	}
}

// wrapper type for Response struct

type filteredBody bytes.Buffer

func (b *filteredBody) Read(byt []byte) (int, error) {
	i, err := (*bytes.Buffer)(b).Read(byt)
	return i, err
}

func (b filteredBody) Close() error {
	return nil
}
