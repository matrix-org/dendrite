// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package internal

import (
	"github.com/element-hq/dendrite/setup/config"
	"github.com/sirupsen/logrus"
)

// SetupHookLogging configures the logging hooks defined in the configuration.
// If something fails here it means that the logging was improperly configured,
// so we just exit with the error
func SetupHookLogging(hooks []config.LogrusHook) {
	logrus.SetReportCaller(true)
	for _, hook := range hooks {
		// Check we received a proper logging level
		level, err := logrus.ParseLevel(hook.Level)
		if err != nil {
			logrus.Fatalf("Unrecognised logging level %s: %q", hook.Level, err)
		}

		// Perform a first filter on the logs according to the lowest level of all
		// (Eg: If we have hook for info and above, prevent logrus from processing debug logs)
		if logrus.GetLevel() < level {
			logrus.SetLevel(level)
		}

		switch hook.Type {
		case "file":
			checkFileHookParams(hook.Params)
			setupFileHook(hook, level)
		default:
			logrus.Fatalf("Unrecognised logging hook type: %s", hook.Type)
		}
	}
}
