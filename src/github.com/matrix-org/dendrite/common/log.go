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

package common

import (
	"os"
	"path"
	"path/filepath"

	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dugong"
	"github.com/sirupsen/logrus"
)

type utcFormatter struct {
	logrus.Formatter
}

func (f utcFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	entry.Time = entry.Time.UTC()
	return f.Formatter.Format(entry)
}

// Logrus hook which wraps another hook and filters log entries according to their level.
// (Note that we cannot use solely logrus.SetLevel, because Dendrite supports multiple
// levels of logging at the same time.)
type logLevelHook struct {
	level logrus.Level
	logrus.Hook
}

// Levels returns all the levels supported by this hook.
func (h *logLevelHook) Levels() []logrus.Level {
	levels := make([]logrus.Level, 0)

	for _, level := range logrus.AllLevels {
		if level <= h.level {
			levels = append(levels, level)
		}
	}

	return levels
}

// SetupStdLogging configures the logging format to standard output. Typically, it is called when the config is not yet loaded.
func SetupStdLogging() {
	logrus.SetFormatter(&utcFormatter{
		&logrus.TextFormatter{
			TimestampFormat:  "2006-01-02T15:04:05.000000000Z07:00",
			FullTimestamp:    true,
			DisableColors:    false,
			DisableTimestamp: false,
			DisableSorting:   false,
		},
	})
}

// SetupHookLogging configures the logging hooks defined in the configuration.
// If something fails here it means that the logging was improperly configured,
// so we just exit with the error
func SetupHookLogging(hooks []config.LogrusHook, componentName string) {
	for _, hook := range hooks {

		// Check we received a proper logging level
		level, err := logrus.ParseLevel(hook.Level)
		if err != nil {
			logrus.Fatalf("Unrecognized logging level %s: %q", hook.Level, err)
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
		default:
			logrus.Fatalf("Unrecognized logging hook type: %s", hook.Type)
		}
	}
}

// File type hooks should be provided a path to a directory to store log files
func checkFileHookParams(params map[string]interface{}) {
	path, ok := params["path"]
	if !ok {
		logrus.Fatalf("Expecting a parameter \"path\" for logging hook of type \"file\"")
	}

	if _, ok := path.(string); !ok {
		logrus.Fatalf("Parameter \"path\" for logging hook of type \"file\" should be a string")
	}
}

// Add a new FSHook to the logger. Each component will log in its own file
func setupFileHook(hook config.LogrusHook, level logrus.Level, componentName string) {
	dirPath := (hook.Params["path"]).(string)
	fullPath := filepath.Join(dirPath, componentName+".log")

	if err := os.MkdirAll(path.Dir(fullPath), os.ModePerm); err != nil {
		logrus.Fatalf("Couldn't create directory %s: %q", path.Dir(fullPath), err)
	}

	logrus.AddHook(&logLevelHook{
		level,
		dugong.NewFSHook(
			fullPath,
			&utcFormatter{
				&logrus.TextFormatter{
					TimestampFormat:  "2006-01-02T15:04:05.000000000Z07:00",
					DisableColors:    true,
					DisableTimestamp: false,
					DisableSorting:   false,
				},
			},
			&dugong.DailyRotationSchedule{GZip: true},
		),
	})
}
