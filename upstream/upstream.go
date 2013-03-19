package upstream

import (
	"bytes"
	"fmt"
	"github.com/ngmoco/falcore"
	"io"
	"net"
	"net/http"
	"time"
)

type passThruReadCloser struct {
	io.Reader
	io.Closer
}

type connWrapper struct {
	conn    net.Conn
	timeout time.Duration
}

func (cw *connWrapper) Write(b []byte) (int, error) {
	if err := cw.conn.SetDeadline(time.Now().Add(cw.timeout)); err != nil {
		return 0, err
	}
	return cw.conn.Write(b)
}
func (cw *connWrapper) Read(b []byte) (n int, err error)   { return cw.conn.Read(b) }
func (cw *connWrapper) Close() error                       { return cw.conn.Close() }
func (cw *connWrapper) LocalAddr() net.Addr                { return cw.conn.LocalAddr() }
func (cw *connWrapper) RemoteAddr() net.Addr               { return cw.conn.RemoteAddr() }
func (cw *connWrapper) SetDeadline(t time.Time) error      { return cw.conn.SetDeadline(t) }
func (cw *connWrapper) SetReadDeadline(t time.Time) error  { return cw.conn.SetReadDeadline(t) }
func (cw *connWrapper) SetWriteDeadline(t time.Time) error { return cw.conn.SetWriteDeadline(t) }

type Upstream struct {
	// The upstream host to connect to
	Host string
	// The port on the upstream host
	Port int
	// Default 60 seconds
	Timeout time.Duration
	// Will ignore https on the incoming request and always upstream http
	ForceHttp bool
	// Ping URL Path-only for checking upness
	PingPath string

	transport *http.Transport
	host      string
	tcpaddr   *net.TCPAddr
}

func NewUpstream(host string, port int, forceHttp bool) *Upstream {
	u := new(Upstream)
	u.Host = host
	u.Port = port
	u.ForceHttp = forceHttp
	ips, err := net.LookupIP(host)
	var ip net.IP = nil
	for i := range ips {
		ip = ips[i].To4()
		if ip != nil {
			break
		}
	}
	if err == nil && ip != nil {
		u.tcpaddr = &net.TCPAddr{}
		u.tcpaddr.Port = port
		u.tcpaddr.IP = ip
	} else {
		falcore.Warn("Can't get IP addr for %v: %v", host, err)
	}
	u.Timeout = 60 * time.Second
	u.host = fmt.Sprintf("%v:%v", u.Host, u.Port)

	u.transport = &http.Transport{}
	// This dial ignores the addr passed in and dials based on the upstream host and port
	u.transport.Dial = func(n, addr string) (c net.Conn, err error) {
		falcore.Fine("Dialing connection to %v", u.tcpaddr)
		var ctcp *net.TCPConn
		ctcp, err = net.DialTCP("tcp4", nil, u.tcpaddr)
		if err != nil {
			falcore.Error("Dial Failed: %v", err)
			return
		}
		c = &connWrapper{conn: ctcp, timeout: u.Timeout}
		return
	}
	u.transport.MaxIdleConnsPerHost = 15
	return u
}

// Alter the number of connections to multiplex with
func (u *Upstream) SetPoolSize(size int) {
	u.transport.MaxIdleConnsPerHost = size
}

func (u *Upstream) FilterRequest(request *falcore.Request) (res *http.Response) {
	var err error
	req := request.HttpRequest

	// Force the upstream to use http 
	if u.ForceHttp || req.URL.Scheme == "" {
		req.URL.Scheme = "http"
		req.URL.Host = req.Host
	}
	before := time.Now()
	req.Header.Set("Connection", "Keep-Alive")
	var upstrRes *http.Response
	upstrRes, err = u.transport.RoundTrip(req)
	diff := falcore.TimeDiff(before, time.Now())
	if err == nil {
		// Copy response over to new record.  Remove connection noise.  Add some sanity.
		res = falcore.SimpleResponse(req, upstrRes.StatusCode, nil, "")
		if upstrRes.ContentLength > 0 && upstrRes.Body != nil {
			res.ContentLength = upstrRes.ContentLength
			res.Body = upstrRes.Body
		} else if upstrRes.ContentLength == 0 && upstrRes.Body != nil {
			// Any bytes?
			var testBuf [1]byte
			n, _ := io.ReadFull(upstrRes.Body, testBuf[:])
			if n == 1 {
				// Yes there are.  Chunked it is.
				res.TransferEncoding = []string{"chunked"}
				res.ContentLength = -1
				rc := &passThruReadCloser{
					io.MultiReader(bytes.NewBuffer(testBuf[:]), upstrRes.Body),
					upstrRes.Body,
				}

				res.Body = rc
			}
		} else if upstrRes.Body != nil {
			res.Body = upstrRes.Body
			res.ContentLength = -1
			res.TransferEncoding = []string{"chunked"}
		}
		// Copy over headers with a few exceptions
		res.Header = make(http.Header)
		for hn, hv := range upstrRes.Header {
			switch hn {
			case "Content-Length":
			case "Connection":
			case "Transfer-Encoding":
			default:
				res.Header[hn] = hv
			}
		}
	} else {
		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			falcore.Error("%s Upstream Timeout error: %v", request.ID, err)
			res = falcore.SimpleResponse(req, 504, nil, "Gateway Timeout\n")
			request.CurrentStage.Status = 2 // Fail
		} else {
			falcore.Error("%s Upstream error: %v", request.ID, err)
			res = falcore.SimpleResponse(req, 502, nil, "Bad Gateway\n")
			request.CurrentStage.Status = 2 // Fail
		}
	}
	falcore.Debug("%s [%s] [%s] %s s=%d Time=%.4f", request.ID, req.Method, u.host, req.URL, res.StatusCode, diff)
	return
}

func (u *Upstream) ping() (up bool, ok bool) {
	if u.PingPath != "" {
		// the url must be syntactically valid for this to work but the host will be ignored because we
		// are overriding the connection always
		request, err := http.NewRequest("GET", "http://localhost"+u.PingPath, nil)
		request.Header.Set("Connection", "Keep-Alive") // not sure if this should be here for a ping
		if err != nil {
			falcore.Error("Bad Ping request: %v", err)
			return false, true
		}
		res, err := u.transport.RoundTrip(request)

		if err != nil {
			falcore.Error("Failed Ping to %v:%v: %v", u.Host, u.Port, err)
			return false, true
		} else {
			res.Body.Close()
		}
		if res.StatusCode == 200 {
			return true, true
		}
		falcore.Error("Failed Ping to %v:%v: %v", u.Host, u.Port, res.Status)
		// bad status
		return false, true
	}
	return false, false
}
