//go:build unix
// +build unix

package base

// OS import to automatically raise file descriptor limit.
import _ "os"

func platformSanityChecks() {
	// Nothing to do yet.
}
