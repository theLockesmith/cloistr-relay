//go:build !linux

package storage

import (
	"syscall"
	"time"
)

func tcpUserTimeoutControl(_ time.Duration) func(string, string, syscall.RawConn) error {
	return nil
}
