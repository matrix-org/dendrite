// Copyright 2017 Vector Creations Ltd
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

package internal

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"

	"github.com/rs/zerolog"
	log "github.com/rs/zerolog/log"

	"github.com/matrix-org/dendrite/setup/config"
)

// logrus is using a global variable when we're using `logrus.AddHook`
// this unfortunately results in us adding the same hook multiple times.
// This map ensures we only ever add one level hook.
var stdLevelLogAdded = make(map[zerolog.Level]bool)
var levelLogAddedMu = &sync.Mutex{}

type utcFormatter struct {
	zerolog.Formatter
}

func (f utcFormatter) Format(entry *zerolog.Event) ([]byte, error) {
	return f.Format(entry)
}

// Logrus hook which wraps another hook and filters log entries according to their level.
// (Note that we cannot use solely logrus.SetLevel, because Dendrite supports multiple
// levels of logging at the same time.)
type logLevelHook struct {
	level zerolog.Level
	zerolog.Hook
}

// Levels returns all the levels supported by this hook.
func (h *logLevelHook) Levels() []zerolog.Level {
	levels := make([]zerolog.Level, 0)
	var level zerolog.Level = -1
	for level <= 5 {
		if level <= h.level {
			levels = append(levels, level)
		}
		level = level + 1
	}

	return levels
}

// callerPrettyfier is a function that given a runtime.Frame object, will
// extract the calling function's name and file, and return them in a nicely
// formatted way
func callerPrettyfier(f *runtime.Frame) (string, string) {
	// Retrieve just the function name
	s := strings.Split(f.Function, ".")
	funcname := s[len(s)-1]

	// Append a newline + tab to it to move the actual log content to its own line
	funcname += "\n\t"

	// Use a shortened file path which just has the filename to avoid having lots of redundant
	// directories which contribute significantly to overall log sizes!
	filename := fmt.Sprintf(" [%s:%d]", path.Base(f.File), f.Line)

	return funcname, filename
}

// SetupPprof starts a pprof listener. We use the DefaultServeMux here because it is
// simplest, and it gives us the freedom to run pprof on a separate port.
func SetupPprof() {
	if hostPort := os.Getenv("PPROFLISTEN"); hostPort != "" {
		logrus.Warn("Starting pprof on ", hostPort)
		go func() {
			logrus.WithError(http.ListenAndServe(hostPort, nil)).Error("Failed to setup pprof listener")
		}()
	}
}

// SetupStdLogging configures the logging format to standard output. Typically, it is called when the config is not yet loaded.
func SetupStdLogging() {
	levelLogAddedMu.Lock()
	defer levelLogAddedMu.Unlock()
	//zerolog.SetReportCaller(true)
	//TimeFieldFormat => dont need
	//DisableColors => dont need
	//DisableTimestamp => dont need
	//Quo0teEmptyFields => dont need
	//callprettyfier => what?
	/*zerolog.Formatter(&utcFormatter{
		&zerolog.TextFormatter{
			TimestampFormat:  "2006-01-02T15:04:05.000000000Z07:00",
			FullTimestamp:    true,
			DisableColors:    false,
			DisableTimestamp: false,
			QuoteEmptyFields: true,
			CallerPrettyfier: callerPrettyfier,
		},
	})*/
}

// File type hooks should be provided a path to a directory to store log files
func checkFileHookParams(params map[string]interface{}) {
	path, ok := params["path"]
	if !ok {
		log.Fatal().Msg("Expecting a parameter \"path\" for logging hook of type \"file\"")
	}

	if _, ok := path.(string); !ok {
		log.Fatal().Msg("Parameter \"path\" for logging hook of type \"file\" should be a string")
	}
}

// Add a new FSHook to the logger. Each component will log in its own file
func setupFileHook(hook config.LogHook, level zerolog.Level) {
	dirPath := (hook.Params["path"]).(string)
	fullPath := filepath.Join(dirPath, "dendrite.log")

	if err := os.MkdirAll(path.Dir(fullPath), os.ModePerm); err != nil {
		log.Fatal().Msgf("Couldn't create directory %s: %q", path.Dir(fullPath), err)
	}
	/*
		log.Hook(&logLevelHook{
			level,
			dugong.NewFSHook(
				fullPath,
				&utcFormatter{log.Logger},
				&dugong.DailyRotationSchedule{GZip: true},
			),
		})*/
	/// TODO: Dugong FSHook!
}

// CloseAndLogIfError Closes io.Closer and logs the error if any
func CloseAndLogIfError(ctx context.Context, closer io.Closer, message string) {
	if closer == nil {
		return
	}
	err := closer.Close()
	if ctx == nil {
		ctx = context.TODO()
	}
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error(message)
	}
}
