package healthcheck

import (
	"syscall"
)

// bindToDevice returns a Control function that binds a socket to a specific network interface
func bindToDevice(iface string) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		var err2 error
		err := c.Control(func(fd uintptr) {
			err2 = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
		})
		if err != nil {
			return err
		}
		return err2
	}
}
