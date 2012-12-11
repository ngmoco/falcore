// +build !windows

package falcore

import (
	"net"
	"syscall"
)

// only valid on non-windows
func (srv *Server) setupNonBlockingListener(err error, l *net.TCPListener) error {
	// FIXME: File() returns a copied pointer.  we're leaking it.  probably doesn't matter
	if srv.listenerFile, err = l.File(); err != nil {
		return err
	}
	fd := int(srv.listenerFile.Fd())
	if e := syscall.SetNonblock(fd, true); e != nil {
		return e
	}
	if srv.sendfile {
		if e := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, srv.sockOpt, 1); e != nil {
			return e
		}
	}
	return nil
}

func (srv *Server) cycleNonBlock(c net.Conn) {
	if srv.sendfile {
		if tcpC, ok := c.(*net.TCPConn); ok {
			if f, err := tcpC.File(); err == nil {
				// f is a copy.  must be closed
				defer f.Close()
				fd := int(f.Fd())
				// Disable TCP_CORK/TCP_NOPUSH
				syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, srv.sockOpt, 0)
				// For TCP_NOPUSH, we need to force flush
				c.Write([]byte{})
				// Re-enable TCP_CORK/TCP_NOPUSH
				syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, srv.sockOpt, 1)
			}
		}
	}
}
