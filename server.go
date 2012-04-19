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
	"reflect"
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
	if runtime.GOOS != "windows" {
		if srv.listenerFile, err = l.File(); err != nil {
			return err
		}
		fd := int(srv.listenerFile.Fd())
		if e := setupFDNonblock(fd); e != nil {
			return e
		}

		if srv.sendfile {
			if e := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, srv.sockOpt, 1); e != nil {
				return e
			}
		}
	}
	return nil
}

// Calling syscall.SetNonblock using reflection to avoid compile errors
// on windows.  This call is not used on windows as hot restart is not supported.
func setupFDNonblock(fd int) error {
	// if function exists
	if fun := reflect.ValueOf(syscall.SetNonblock); fun.Kind() == reflect.Func {
		// if first argument is an int
		if fun.Type().In(0).Kind() == reflect.Int {
			args := []reflect.Value{reflect.ValueOf(fd), reflect.ValueOf(true)}
			if res := fun.Call(args); len(res) == 1 && !res[0].IsNil() {
				err := res[0].Interface().(error)
				return err
			}
		}
	}
	return nil
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
	srv.stopAccepting <- 1
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

func (srv *Server) handler(c net.Conn) {
	startTime := time.Now()
	defer srv.connectionFinished(c)
	buf := bufio.NewReaderSize(c, 8192)
	var err error
	var req *http.Request
	// no keepalive (for now)
	reqCount := 0
	keepAlive := true
	for err == nil && keepAlive {
		if req, err = http.ReadRequest(buf); err == nil {
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
		} else {
			// EOF is socket closed
			if err != io.ErrUnexpectedEOF {
				Error("%s %v ERROR reading request: %v", srv.serverLogPrefix(), c.RemoteAddr(), err)
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

func (srv *Server) connectionFinished(c net.Conn) {
	c.Close()
	srv.handlerWaitGroup.Done()
}
