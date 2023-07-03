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
	"os"

	"github.com/rs/zerolog"
	log "github.com/rs/zerolog/log"

	"github.com/matrix-org/dendrite/setup/config"
)

// SetupHookLogging configures the logging hooks defined in the configuration.
// If something fails here it means that the logging was improperly configured,
// so we just exit with the error
func SetupHookLogging(hooks []config.LogHook) {
	levelLogAddedMu.Lock()
	defer levelLogAddedMu.Unlock()
	for _, hook := range hooks {
		// Check we received a proper logging level
		level, err := zerolog.ParseLevel(hook.Level)
		if err != nil {
			log.Fatal().Msgf("Unrecognised logging level %s: %q", hook.Level, err)
		}

		// Perform a first filter on the logs according to the lowest level of all
		// (Eg: If we have hook for info and above, prevent logrus from processing debug logs)
		if log.Logger.GetLevel() < level {
			log.Logger = log.Logger.Level(level)
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
			log.Fatal().Msgf("Unrecognised logging hook type: %s", hook.Type)
		}
	}
	setupStdLogHook(zerolog.InfoLevel)
	// Hooks are now configured for stdout/err, so throw away the default logger output
	log.Logger = log.Logger.Output(io.Discard)
}

func checkSyslogHookParams(params map[string]interface{}) {
	addr, ok := params["address"]
	if !ok {
		log.Fatal().Msg("Expecting a parameter \"address\" for logging hook of type \"syslog\"")
	}

	if _, ok := addr.(string); !ok {
		log.Fatal().Msg("Parameter \"address\" for logging hook of type \"syslog\" should be a string")
	}

	proto, ok2 := params["protocol"]
	if !ok2 {
		log.Fatal().Msg("Expecting a parameter \"protocol\" for logging hook of type \"syslog\"")
	}

	if _, ok2 := proto.(string); !ok2 {
		log.Fatal().Msg("Parameter \"protocol\" for logging hook of type \"syslog\" should be a string")
	}

}

func setupStdLogHook(level zerolog.Level) {
	if stdLevelLogAdded[level] {
		return
	}
	//log.Logger.Hook(&logLevelHook{level, stdemuxerhook.New(log.Logger)})
	//log.Logger.Hook(&logLevelHook{level, zerolog.New(os.Stdout)})
	log.Logger = zerolog.New(os.Stdout)
	stdLevelLogAdded[level] = true
}

func setupSyslogHook(hook config.LogHook, level zerolog.Level) {
	// rewrite to:
	// https://stackoverflow.com/questions/73064915/logging-to-syslog-file-with-zerolog-golang
	/*syslogHook, err := lSyslog.NewSyslogHook(hook.Params["protocol"].(string), hook.Params["address"].(string), syslog.LOG_INFO, "dendrite")
	if err == nil {
		log.Logger.Hook(&logLevelHook{level, syslogHook})
	}*/
}
