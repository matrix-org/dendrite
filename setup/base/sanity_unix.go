//go:build linux || darwin || netbsd || freebsd || openbsd || solaris || dragonfly || aix
// +build linux darwin netbsd freebsd openbsd solaris dragonfly aix

package base

import (
	"syscall"

	"github.com/sirupsen/logrus"
)

func platformSanityChecks() {
	// Dendrite needs a relatively high number of file descriptors in order
	// to function properly, particularly when federating with lots of servers.
	// If we run out of file descriptors, we might run into problems accessing
	// PostgreSQL amongst other things. Complain at startup if we think the
	// number of file descriptors is too low.
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil && rLimit.Cur < 65535 {
		logrus.Warnf("IMPORTANT: Process file descriptor limit is currently %d, it is recommended to raise the limit for Dendrite to at least 65535 to avoid issues", rLimit.Cur)
	}
}
