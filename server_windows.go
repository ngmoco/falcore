// +build windows
package falcore

import (
	"net"
)

// only valid on non-windows
func SetupNonBlockingListener(srv *Server, err error, l *net.TCPListener) error {
	return nil
}
