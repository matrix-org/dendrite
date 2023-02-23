package jetstream

import (
	"github.com/nats-io/nats-server/v2/server"
	"github.com/sirupsen/logrus"
)

var _ server.Logger = &LogAdapter{}

type LogAdapter struct {
	entry *logrus.Entry
}

func NewLogAdapter() *LogAdapter {
	return &LogAdapter{
		entry: logrus.StandardLogger().WithField("component", "jetstream"),
	}
}

func (l *LogAdapter) Noticef(format string, v ...interface{}) {
	l.entry.Infof(format, v...)
}

func (l *LogAdapter) Warnf(format string, v ...interface{}) {
	l.entry.Warnf(format, v...)
}

func (l *LogAdapter) Fatalf(format string, v ...interface{}) {
	l.entry.Fatalf(format, v...)
}

func (l *LogAdapter) Errorf(format string, v ...interface{}) {
	l.entry.Errorf(format, v...)
}

func (l *LogAdapter) Debugf(format string, v ...interface{}) {
	l.entry.Debugf(format, v...)
}

func (l *LogAdapter) Tracef(format string, v ...interface{}) {
	l.entry.Tracef(format, v...)
}
