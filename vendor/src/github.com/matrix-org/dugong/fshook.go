package dugong

import (
	"compress/gzip"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// RotationScheduler determines when files should be rotated.
type RotationScheduler interface {
	// ShouldRotate returns true if the file should be rotated. The suffix to apply
	// to the filename is returned as the 2nd arg.
	ShouldRotate() (bool, string)
	// ShouldGZip returns true if the file should be gzipped when it is rotated.
	ShouldGZip() bool
}

// DailyRotationSchedule rotates log files daily. Logs are only rotated
// when midnight passes *whilst the process is running*. E.g: if you run
// the process on Day 4 then stop it and start it on Day 7, no rotation will
// occur when the process starts.
type DailyRotationSchedule struct {
	GZip        bool
	rotateAfter *time.Time
}

var currentTime = time.Now // exclusively for testing

func dayOffset(t time.Time, offsetDays int) time.Time {
	// GoDoc:
	//   The month, day, hour, min, sec, and nsec values may be outside their
	//   usual ranges and will be normalized during the conversion.
	//   For example, October 32 converts to November 1.
	return time.Date(
		t.Year(), t.Month(), t.Day()+offsetDays, 0, 0, 0, 0, t.Location(),
	)
}

func (rs *DailyRotationSchedule) ShouldRotate() (bool, string) {
	now := currentTime()
	if rs.rotateAfter == nil {
		nextRotate := dayOffset(now, 1)
		rs.rotateAfter = &nextRotate
		return false, ""
	}
	if now.After(*rs.rotateAfter) {
		// the suffix should be actually the date of the complete day being logged
		actualDay := dayOffset(*rs.rotateAfter, -1)
		suffix := "." + actualDay.Format("2006-01-02") // YYYY-MM-DD
		nextRotate := dayOffset(now, 1)
		rs.rotateAfter = &nextRotate
		return true, suffix
	}
	return false, ""
}

func (rs *DailyRotationSchedule) ShouldGZip() bool {
	return rs.GZip
}

// NewFSHook makes a logging hook that writes formatted
// log entries to info, warn and error log files. Each log file
// contains the messages with that severity or higher. If a formatter is
// not specified, they will be logged using a JSON formatter. If a
// RotationScheduler is set, the files will be cycled according to its rules.
func NewFSHook(infoPath, warnPath, errorPath string, formatter log.Formatter, rotSched RotationScheduler) log.Hook {
	if formatter == nil {
		formatter = &log.JSONFormatter{}
	}
	hook := &fsHook{
		entries:   make(chan log.Entry, 1024),
		infoPath:  infoPath,
		warnPath:  warnPath,
		errorPath: errorPath,
		formatter: formatter,
		scheduler: rotSched,
	}

	go func() {
		for entry := range hook.entries {
			if err := hook.writeEntry(&entry); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing to logfile: %v\n", err)
			}
			atomic.AddInt32(&hook.queueSize, -1)
		}
	}()

	return hook
}

type fsHook struct {
	entries   chan log.Entry
	queueSize int32
	infoPath  string
	warnPath  string
	errorPath string
	formatter log.Formatter
	scheduler RotationScheduler
}

func (hook *fsHook) Fire(entry *log.Entry) error {
	atomic.AddInt32(&hook.queueSize, 1)
	hook.entries <- *entry
	return nil
}

func (hook *fsHook) writeEntry(entry *log.Entry) error {
	msg, err := hook.formatter.Format(entry)
	if err != nil {
		return nil
	}

	if hook.scheduler != nil {
		if should, suffix := hook.scheduler.ShouldRotate(); should {
			if err := hook.rotate(suffix, hook.scheduler.ShouldGZip()); err != nil {
				return err
			}
		}
	}

	if entry.Level <= log.ErrorLevel {
		if err := logToFile(hook.errorPath, msg); err != nil {
			return err
		}
	}

	if entry.Level <= log.WarnLevel {
		if err := logToFile(hook.warnPath, msg); err != nil {
			return err
		}
	}

	if entry.Level <= log.InfoLevel {
		if err := logToFile(hook.infoPath, msg); err != nil {
			return err
		}
	}

	return nil
}

func (hook *fsHook) Levels() []log.Level {
	return []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
		log.InfoLevel,
	}
}

// rotate all the log files to the given suffix.
// If error path is "err.log" and suffix is "1" then move
// the contents to "err.log1".
// This requires no locking as the goroutine calling this is the same
// one which does the logging. Since we don't hold open a handle to the
// file when writing, a simple Rename is all that is required.
func (hook *fsHook) rotate(suffix string, gzip bool) error {
	for _, fpath := range []string{hook.errorPath, hook.warnPath, hook.infoPath} {
		logFilePath := fpath + suffix
		if err := os.Rename(fpath, logFilePath); err != nil {
			// e.g. because there were no errors in error.log for this day
			fmt.Fprintf(os.Stderr, "Error rotating file %s: %v\n", fpath, err)
			continue // don't try to gzip if we failed to rotate
		}
		if gzip {
			if err := gzipFile(logFilePath); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to gzip file %s: %v\n", logFilePath, err)
			}
		}
	}
	return nil
}

func logToFile(path string, msg []byte) error {
	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer fd.Close()
	_, err = fd.Write(msg)
	return err
}

func gzipFile(fpath string) error {
	reader, err := os.Open(fpath)
	if err != nil {
		return err
	}

	filename := filepath.Base(fpath)
	target := filepath.Join(filepath.Dir(fpath), filename+".gz")
	writer, err := os.Create(target)
	if err != nil {
		return err
	}
	defer writer.Close()

	archiver := gzip.NewWriter(writer)
	archiver.Name = filename
	defer archiver.Close()

	_, err = io.Copy(archiver, reader)
	return err
}
