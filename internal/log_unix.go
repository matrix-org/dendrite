// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

//go:build !windows
// +build !windows

package internal

import (
	"io"
	"log/syslog"

	"github.com/MFAshby/stdemuxerhook"
	"github.com/sirupsen/logrus"
	lSyslog "github.com/sirupsen/logrus/hooks/syslog"

	"github.com/element-hq/dendrite/setup/config"
)

// SetupHookLogging configures the logging hooks defined in the configuration.
// If something fails here it means that the logging was improperly configured,
// so we just exit with the error
func SetupHookLogging(hooks []config.LogrusHook) {
	levelLogAddedMu.Lock()
	defer levelLogAddedMu.Unlock()
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
		case "syslog":
			checkSyslogHookParams(hook.Params)
			setupSyslogHook(hook, level)
		case "std":
			setupStdLogHook(level)
		default:
			logrus.Fatalf("Unrecognised logging hook type: %s", hook.Type)
		}
	}
	setupStdLogHook(logrus.InfoLevel)
	// Hooks are now configured for stdout/err, so throw away the default logger output
	logrus.SetOutput(io.Discard)
}

func checkSyslogHookParams(params map[string]interface{}) {
	addr, ok := params["address"]
	if !ok {
		logrus.Fatalf("Expecting a parameter \"address\" for logging hook of type \"syslog\"")
	}

	if _, ok := addr.(string); !ok {
		logrus.Fatalf("Parameter \"address\" for logging hook of type \"syslog\" should be a string")
	}

	proto, ok2 := params["protocol"]
	if !ok2 {
		logrus.Fatalf("Expecting a parameter \"protocol\" for logging hook of type \"syslog\"")
	}

	if _, ok2 := proto.(string); !ok2 {
		logrus.Fatalf("Parameter \"protocol\" for logging hook of type \"syslog\" should be a string")
	}

}

func setupStdLogHook(level logrus.Level) {
	if stdLevelLogAdded[level] {
		return
	}
	logrus.AddHook(&logLevelHook{level, stdemuxerhook.New(logrus.StandardLogger())})
	stdLevelLogAdded[level] = true
}

func setupSyslogHook(hook config.LogrusHook, level logrus.Level) {
	syslogHook, err := lSyslog.NewSyslogHook(hook.Params["protocol"].(string), hook.Params["address"].(string), syslog.LOG_INFO, "dendrite")
	if err == nil {
		logrus.AddHook(&logLevelHook{level, syslogHook})
	}
}
