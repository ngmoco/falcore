package falcore

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type Server struct {
	Addr             string
	Pipeline         *Pipeline
	listener         net.Listener
	listenerFile     *os.File
	stopAccepting    chan int
	handlerWaitGroup *sync.WaitGroup
	logPrefix        string
	AcceptReady      chan int
	sendfile         bool
	sockOpt          int
	bufferPool       *bufferPool
}

func NewServer(port int, pipeline *Pipeline) *Server {
	s := new(Server)
	s.Addr = fmt.Sprintf(":%v", port)
	s.Pipeline = pipeline
	s.stopAccepting = make(chan int)
	s.AcceptReady = make(chan int, 1)
	s.handlerWaitGroup = new(sync.WaitGroup)
	s.logPrefix = fmt.Sprintf("%d", syscall.Getpid())

	// openbsd/netbsd don't have TCP_NOPUSH so it's likely sendfile will be slower
	// without these socket options, just enable for linux, mac and freebsd.
	// TODO (Graham) windows has TransmitFile zero-copy mechanism, try to use it
	switch runtime.GOOS {
	case "linux":
		s.sendfile = true
		s.sockOpt = 0x3 // syscall.TCP_CORK
	case "freebsd", "darwin":
		s.sendfile = true
		s.sockOpt = 0x4 // syscall.TCP_NOPUSH
	default:
		s.sendfile = false
	}

	// buffer pool for reusing connection bufio.Readers
	s.bufferPool = newBufferPool(100, 8192)

	return s
}

func (srv *Server) FdListen(fd int) error {
	var err error
	srv.listenerFile = os.NewFile(uintptr(fd), "")
	if srv.listener, err = net.FileListener(srv.listenerFile); err != nil {
		return err
	}
	if _, ok := srv.listener.(*net.TCPListener); !ok {
		return errors.New("Broken listener isn't TCP")
	}
	return nil
}

func (srv *Server) socketListen() error {
	var la *net.TCPAddr
	var err error
	if la, err = net.ResolveTCPAddr("tcp", srv.Addr); err != nil {
		return err
	}

	var l *net.TCPListener
	if l, err = net.ListenTCP("tcp", la); err != nil {
		return err
	}
	srv.listener = l
	// setup listener to be non-blocking if we're not on windows.
	// this is required for hot restart to work.
	return srv.setupNonBlockingListener(err, l)
}

func (srv *Server) ListenAndServe() error {
	if srv.Addr == "" {
		srv.Addr = ":http"
	}
	if srv.listener == nil {
		if err := srv.socketListen(); err != nil {
			return err
		}
	}
	return srv.serve()
}

func (srv *Server) SocketFd() int {
	return int(srv.listenerFile.Fd())
}

func (srv *Server) ListenAndServeTLS(certFile, keyFile string) error {
	if srv.Addr == "" {
		srv.Addr = ":https"
	}
	config := &tls.Config{
		Rand:       rand.Reader,
		Time:       time.Now,
		NextProtos: []string{"http/1.1"},
	}

	var err error
	config.Certificates = make([]tls.Certificate, 1)
	config.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	if srv.listener == nil {
		if err := srv.socketListen(); err != nil {
			return err
		}
	}

	srv.listener = tls.NewListener(srv.listener, config)

	return srv.serve()
}

func (srv *Server) StopAccepting() {
	close(srv.stopAccepting)
}

func (srv *Server) Port() int {
	if l := srv.listener; l != nil {
		a := l.Addr()
		if _, p, e := net.SplitHostPort(a.String()); e == nil && p != "" {
			server_port, _ := strconv.Atoi(p)
			return server_port
		}
	}
	return 0
}

func (srv *Server) serve() (e error) {
	var accept = true
	srv.AcceptReady <- 1
	for accept {
		var c net.Conn
		if l, ok := srv.listener.(*net.TCPListener); ok {
			l.SetDeadline(time.Now().Add(3e9))
		}
		c, e = srv.listener.Accept()
		if e != nil {
			if ope, ok := e.(*net.OpError); ok {
				if !(ope.Timeout() && ope.Temporary()) {
					Error("%s SERVER Accept Error: %v", srv.serverLogPrefix(), ope)
				}
			} else {
				Error("%s SERVER Accept Error: %v", srv.serverLogPrefix(), e)
			}
		} else {
			//Trace("Handling!")
			srv.handlerWaitGroup.Add(1)
			go srv.handler(c)
		}
		select {
		case <-srv.stopAccepting:
			accept = false
		default:
		}
	}
	Trace("Stopped accepting, waiting for handlers")
	// wait for handlers
	srv.handlerWaitGroup.Wait()
	return nil
}

func (srv *Server) sentinel(c net.Conn, connClosed chan int) {
	select {
	case <-srv.stopAccepting:
		c.SetReadDeadline(time.Now().Add(3 * time.Second))
	case <-connClosed:
	}
}

func (srv *Server) handler(c net.Conn) {
	startTime := time.Now()
	bpe := srv.bufferPool.take(c)
	defer srv.bufferPool.give(bpe)
	var closeSentinelChan = make(chan int)
	go srv.sentinel(c, closeSentinelChan)
	defer srv.connectionFinished(c, closeSentinelChan)
	var err error
	var req *http.Request
	// no keepalive (for now)
	reqCount := 0
	keepAlive := true
	for err == nil && keepAlive {
		if req, err = http.ReadRequest(bpe.br); err == nil {
			if req.Header.Get("Connection") != "Keep-Alive" {
				keepAlive = false
			}
			request := newRequest(req, c, startTime)
			reqCount++
			var res *http.Response

			pssInit := new(PipelineStageStat)
			pssInit.Name = "server.Init"
			pssInit.StartTime = startTime
			pssInit.EndTime = time.Now()
			request.appendPipelineStage(pssInit)
			// execute the pipeline
			if res = srv.Pipeline.execute(request); res == nil {
				res = SimpleResponse(req, 404, nil, "Not Found")
			}
			// cleanup
			request.startPipelineStage("server.ResponseWrite")
			req.Body.Close()

			// shutting down?
			select {
			case <-srv.stopAccepting:
				keepAlive = false
				res.Close = true
			default:
			}
			// The res.Write omits Content-length on 0 length bodies, and by spec, 
			// it SHOULD. While this is not MUST, it's kinda broken.  See sec 4.4 
			// of rfc2616 and a 200 with a zero length does not satisfy any of the
			// 5 conditions if Connection: keep-alive is set :(
			// I'm forcing chunked which seems to work because I couldn't get the
			// content length to write if it was 0.
			// Specifically, the android http client waits forever if there's no
			// content-length instead of assuming zero at the end of headers. der.
			if res.ContentLength == 0 && len(res.TransferEncoding) == 0 && !((res.StatusCode-100 < 100) || res.StatusCode == 204 || res.StatusCode == 304) {
				res.TransferEncoding = []string{"identity"}
			}
			if res.ContentLength < 0 {
				res.TransferEncoding = []string{"chunked"}
			}

			// write response
			if srv.sendfile {
				res.Write(c)
			} else {
				wbuf := bufio.NewWriter(c)
				res.Write(wbuf)
				wbuf.Flush()
			}
			if res.Body != nil {
				res.Body.Close()
			}
			request.finishPipelineStage()
			request.finishRequest()
			srv.requestFinished(request)
			
			if res.Close {
				keepAlive = false
			}
			
			// Reset the startTime
			// this isn't great since there may be lag between requests; but it's the best we've got
			startTime = time.Now()
		} else {
			// EOF is socket closed
			if nerr, ok := err.(net.Error); err != io.EOF && !(ok && nerr.Timeout()) {
				Error("%s %v ERROR reading request: <%T %v>", srv.serverLogPrefix(), c.RemoteAddr(), err, err)
			}
		}
	}
	//Debug("%s Processed %v requests on connection %v", srv.serverLogPrefix(), reqCount, c.RemoteAddr())
}

func (srv *Server) serverLogPrefix() string {
	return srv.logPrefix
}

func (srv *Server) requestFinished(request *Request) {
	if srv.Pipeline.RequestDoneCallback != nil {
		// Don't block the connecion for this
		go srv.Pipeline.RequestDoneCallback.FilterRequest(request)
	}
}

func (srv *Server) connectionFinished(c net.Conn, closeChan chan int) {
	c.Close()
	close(closeChan)
	srv.handlerWaitGroup.Done()
}
