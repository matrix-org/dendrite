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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/matrix-org/dugong"
	"github.com/mgutz/ansi"
	"github.com/sirupsen/logrus"
)

type dendriteFormatter struct {
	logrus.TextFormatter
}

func (f dendriteFormatter) Format(entry *logrus.Entry) (format []byte, err error) {
	entry.Time = entry.Time.UTC()

	prefix, ok := entry.Data["prefix"].(string)
	if !ok {
		return f.TextFormatter.Format(entry)
	}

	prefix = strings.ToUpper(prefix)

	if !f.TextFormatter.DisableColors {
		prefix = ansi.Color(prefix, "white+b")
	}

	entry.Message = fmt.Sprintf("%s: %s\t", prefix, entry.Message)

	// Generate the formatted log without the prefix as a field
	// Use a copy of the entry so the same entry isn't altered by multiple
	// fields at the same time
	entryCpy := *entry
	// Go doesn't perform deep copies, so the fields have to be manually
	// copied
	fields := make(logrus.Fields)
	for k, v := range entry.Data {
		if k != "prefix" {
			fields[k] = v
		}
	}
	entryCpy.Data = fields
	format, err = f.TextFormatter.Format(&entryCpy)

	return
}

// SetupLogging configures the logging format and destination(s).
func SetupLogging(logDir string) {
	logrus.SetFormatter(dendriteFormatter{
		logrus.TextFormatter{
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
			dendriteFormatter{
				logrus.TextFormatter{
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
