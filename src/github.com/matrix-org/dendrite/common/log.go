package common

import (
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/matrix-org/dugong"
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
	formatter := &utcFormatter{
		&logrus.TextFormatter{
			TimestampFormat:  "2006-01-02T15:04:05.000000000Z07:00",
			DisableColors:    true,
			DisableTimestamp: false,
			DisableSorting:   false,
		},
	}
	if logDir != "" {
		_ = os.Mkdir(logDir, os.ModePerm)
		logrus.AddHook(dugong.NewFSHook(
			filepath.Join(logDir, "info.log"),
			filepath.Join(logDir, "warn.log"),
			filepath.Join(logDir, "error.log"),
			formatter,
			&dugong.DailyRotationSchedule{GZip: true},
		))
	} else {
		logrus.SetFormatter(formatter)
	}
}
