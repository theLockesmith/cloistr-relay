//go:build linux

package storage

import (
	"syscall"
	"time"
)

func tcpUserTimeoutControl(d time.Duration) func(string, string, syscall.RawConn) error {
	ms := int(d.Milliseconds())
	return func(_, _ string, c syscall.RawConn) error {
		var seterr error
		err := c.Control(func(fd uintptr) {
			seterr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, 18, ms)
		})
		if err != nil {
			return err
		}
		return seterr
	}
}
