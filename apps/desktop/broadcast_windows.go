//go:build windows

package main

import (
	"net"
	"syscall"
)

// enableBroadcast lets the announcement reach 255.255.255.255, which Windows
// refuses unless SO_BROADCAST is set.
func enableBroadcast(c *net.UDPConn) {
	raw, err := c.SyscallConn()
	if err != nil {
		return
	}
	_ = raw.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	})
}
