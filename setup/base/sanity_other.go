//go:build !linux && !darwin && !netbsd && !freebsd && !openbsd && !solaris && !dragonfly && !aix
// +build !linux,!darwin,!netbsd,!freebsd,!openbsd,!solaris,!dragonfly,!aix

package base

func platformSanityChecks() {
	// Nothing to do yet.
}
