package internal

import (
	"fmt"
	"strings"
)

// the final version string
var version string

// -ldflags "-X github.com/matrix-org/dendrite/internal.branch=master"
var branch string

// -ldflags "-X github.com/matrix-org/dendrite/internal.build=alpha"
var build string

const (
	VersionMajor = 0
	VersionMinor = 10
	VersionPatch = 8
	VersionTag   = "" // example: "rc1"
)

func VersionString() string {
	return version
}

func init() {
	version = fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
	if VersionTag != "" {
		version += "-" + VersionTag
	}
	parts := []string{}
	if build != "" {
		parts = append(parts, build)
	}
	if branch != "" {
		parts = append(parts, branch)
	}
	if len(parts) > 0 {
		version += "+" + strings.Join(parts, ".")
	}
}
