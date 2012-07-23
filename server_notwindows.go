// +build !windows

package falcore

import (
	"net"
	"syscall"
)

// only valid on non-windows
func (srv *Server) setupNonBlockingListener(srv *Server, err error, l *net.TCPListener) error {
	if srv.listenerFile, err = l.File(); err != nil {
		return err
	}
	fd := int(srv.listenerFile.Fd())
    if e := syscall.SetNonblock(fd); e != nil {
        return e
    }
	if srv.sendfile {
		if e := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, srv.sockOpt, 1); e != nil {
			return e
		}
	}
}

