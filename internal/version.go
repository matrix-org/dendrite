package internal

import "fmt"

// -ldflags "-X github.com/matrix-org/dendrite/internal.branch=master"
var branch string

// -ldflags "-X github.com/matrix-org/dendrite/internal.build=alpha"
var build string

const (
	VersionMajor = 0
	VersionMinor = 1
	VersionPatch = 0
	VersionTag   = "" // example: "rc1"
)

func VersionString() string {
	version := fmt.Sprintf("%d.%d.%d%s", VersionMajor, VersionMinor, VersionPatch, VersionTag)
	if branch != "" {
		version += fmt.Sprintf("-%s", branch)
	}
	if build != "" {
		version += fmt.Sprintf("+%s", build)
	}
	return version
}
