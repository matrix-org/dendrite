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
	"path/filepath"

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

// SetupLogging configures the logging format and destination(s).
func SetupLogging(logDir string) {
	logrus.SetFormatter(&utcFormatter{
		&logrus.TextFormatter{
			TimestampFormat:  "2006-01-02T15:04:05.000000000Z07:00",
			FullTimestamp:    true,
			DisableColors:    false,
			DisableTimestamp: false,
			DisableSorting:   false,
		},
	})
	if logDir != "" {
		_ = os.Mkdir(logDir, os.ModePerm)
		logrus.AddHook(dugong.NewFSHook(
			filepath.Join(logDir, "info.log"),
			filepath.Join(logDir, "warn.log"),
			filepath.Join(logDir, "error.log"),
			&utcFormatter{
				&logrus.TextFormatter{
					TimestampFormat:  "2006-01-02T15:04:05.000000000Z07:00",
					DisableColors:    true,
					DisableTimestamp: false,
					DisableSorting:   false,
				},
			},
			&dugong.DailyRotationSchedule{GZip: true},
		))
	}
}
