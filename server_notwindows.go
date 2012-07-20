// +build !windows

package falcore

import (
	"net"
	"reflect"
	"syscall"
)

// only valid on non-windows
func SetupNonBlockingListener(srv *Server, err error, l *net.TCPListener) error {
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
