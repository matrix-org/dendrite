package internal

import (
	"fmt"
	"runtime/debug"
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
	VersionMinor = 13
	VersionPatch = 1
	VersionTag   = "" // example: "rc1"

	gitRevLen = 7 // 7 matches the displayed characters on github.com
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

	defer func() {
		if len(parts) > 0 {
			version += "+" + strings.Join(parts, ".")
		}
	}()

	// Try to get the revision Dendrite was build from.
	// If we can't, e.g. Dendrite wasn't built (go run) or no VCS version is present,
	// we just use the provided version above.
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			revLen := len(setting.Value)
			if revLen >= gitRevLen {
				parts = append(parts, setting.Value[:gitRevLen])
			} else {
				parts = append(parts, setting.Value[:revLen])
			}
			break
		}
	}
}
