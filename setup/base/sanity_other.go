//go:build !linux && !darwin && !netbsd && !freebsd && !openbsd && !solaris && !dragonfly && !aix
// +build !linux,!darwin,!netbsd,!freebsd,!openbsd,!solaris,!dragonfly,!aix

package base

func PlatformSanityChecks() {
	// Nothing to do yet.
}
