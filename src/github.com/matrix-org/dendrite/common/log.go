package common

import (
	"os"
	"path/filepath"

	"github.com/Sirupsen/logrus"
	"github.com/matrix-org/dugong"
)

// SetupLogging configures the logging format and destination(s).
func SetupLogging(logDir string) {
	_ = os.Mkdir(logDir, os.ModePerm)
	logrus.AddHook(dugong.NewFSHook(
		filepath.Join(logDir, "info.log"),
		filepath.Join(logDir, "warn.log"),
		filepath.Join(logDir, "error.log"),
		&logrus.TextFormatter{
			TimestampFormat:  "2006-01-02 15:04:05.000000",
			DisableColors:    true,
			DisableTimestamp: false,
			DisableSorting:   false,
		}, &dugong.DailyRotationSchedule{GZip: true},
	))
}
