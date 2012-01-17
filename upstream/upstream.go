package upstream

import (
	"http"
	"os"
	"net"
	"fmt"
	"time"
	"falcore"
)

type Upstream struct {
	// The upstream host to connect to
	Host string
	// The port on the upstream host
	Port int
	// Default 60 seconds
	Timeout int64
	// Will ignore https on the incoming request and always upstream http
	ForceHttp bool

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
		u.tcpaddr = new(net.TCPAddr)
		u.tcpaddr.Port = port
		u.tcpaddr.IP = ip
	} else {
		falcore.Warn("Can't get IP addr for %v: %v", host, err)
	}
	u.Timeout = 60e9
	u.host = fmt.Sprintf("%v:%v", u.Host, u.Port)

	u.transport = new(http.Transport)
	u.transport.Dial = func(n, addr string) (c net.Conn, err os.Error) {
		falcore.Debug("Dialing connection to %v", u.tcpaddr)
		var ctcp *net.TCPConn
		ctcp, err = net.DialTCP("tcp4", nil, u.tcpaddr)
		if ctcp != nil {
			ctcp.SetTimeout(u.Timeout)
		}
		if err != nil {
			falcore.Error("Dial Failed: %v", err)
		}
		return ctcp, err
	}
	u.transport.MaxIdleConnsPerHost = 15
	return u
}

// Alter the number of connections to multiplex with
func (u *Upstream) SetPoolSize(size int) {
	u.transport.MaxIdleConnsPerHost = size
}

func (u *Upstream) FilterRequest(request *falcore.Request) (res *http.Response) {
	var err os.Error
	req := request.HttpRequest

	// Force the upstream to use http 
	if u.ForceHttp || req.URL.Scheme == "" {
		req.URL.Scheme = "http"
		req.URL.Host = req.Host
	}
	before := time.Nanoseconds()
	req.Header.Set("Connection", "Keep-Alive")
	res, err = u.transport.RoundTrip(req)
	diff := falcore.TimeDiff(before, time.Nanoseconds())
	if err != nil {
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
	falcore.Debug("%s [%s] [%s%s] s=%d Time=%.4f", request.ID, req.Method, u.host, req.RawURL, res.StatusCode, diff)
	return
}
