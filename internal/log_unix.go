// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows
// +build !windows

package internal

import (
	"io"
	"log/syslog"

	"github.com/MFAshby/stdemuxerhook"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/sirupsen/logrus"
	lSyslog "github.com/sirupsen/logrus/hooks/syslog"
)

// SetupHookLogging configures the logging hooks defined in the configuration.
// If something fails here it means that the logging was improperly configured,
// so we just exit with the error
func SetupHookLogging(hooks []config.LogrusHook, componentName string) {
	stdLogAdded := false
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
			setupFileHook(hook, level, componentName)
		case "syslog":
			checkSyslogHookParams(hook.Params)
			setupSyslogHook(hook, level, componentName)
		case "std":
			setupStdLogHook(level)
			stdLogAdded = true
		default:
			logrus.Fatalf("Unrecognised logging hook type: %s", hook.Type)
		}
	}
	if !stdLogAdded {
		setupStdLogHook(logrus.InfoLevel)
	}
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
	logrus.AddHook(&logLevelHook{level, stdemuxerhook.New(logrus.StandardLogger())})
}

func setupSyslogHook(hook config.LogrusHook, level logrus.Level, componentName string) {
	syslogHook, err := lSyslog.NewSyslogHook(hook.Params["protocol"].(string), hook.Params["address"].(string), syslog.LOG_INFO, componentName)
	if err == nil {
		logrus.AddHook(&logLevelHook{level, syslogHook})
	}
}
