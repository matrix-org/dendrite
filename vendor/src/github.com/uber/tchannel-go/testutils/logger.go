// Copyright (c) 2015 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package testutils

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uber/tchannel-go"

	"github.com/uber-go/atomic"
)

// writer is shared between multiple loggers, and serializes acccesses to
// the underlying buffer.
type writer struct {
	sync.Mutex
	buf *bytes.Buffer
}

// testLogger is a logger that writes all output to a buffer, and can report
// the logs if the test has failed.
type testLogger struct {
	t      testing.TB
	fields tchannel.LogFields
	w      *writer
}

type errorLoggerState struct {
	matchCount []atomic.Uint32
}

type errorLogger struct {
	tchannel.Logger
	t testing.TB
	v *LogVerification
	s *errorLoggerState
}

func newWriter() *writer {
	return &writer{buf: &bytes.Buffer{}}
}

func (w *writer) withLock(f func(*bytes.Buffer)) {
	w.Lock()
	f(w.buf)
	w.Unlock()
}

// Matches returns true if the message and fields match the filter.
func (f LogFilter) Matches(msg string, fields tchannel.LogFields) bool {
	// First check the message and ensure it contains Filter
	if !strings.Contains(msg, f.Filter) {
		return false
	}

	// if there are no field filters, then the message match is enough.
	if len(f.FieldFilters) == 0 {
		return true
	}

	fieldsMap := make(map[string]interface{})
	for _, field := range fields {
		fieldsMap[field.Key] = field.Value
	}

	for k, filter := range f.FieldFilters {
		value, ok := fieldsMap[k]
		if !ok {
			return false
		}

		if !strings.Contains(fmt.Sprint(value), filter) {
			return false
		}
	}

	return true
}
func newTestLogger(t testing.TB) testLogger {
	return testLogger{t, nil, newWriter()}
}

func (l testLogger) Enabled(level tchannel.LogLevel) bool {
	return true
}

func (l testLogger) log(prefix string, msg string) {
	logLine := fmt.Sprintf("%s [%v] %v %v\n", time.Now().Format("15:04:05.000000"), prefix, msg, l.Fields())
	l.w.withLock(func(w *bytes.Buffer) {
		w.WriteString(logLine)
	})
}

func (l testLogger) Fatal(msg string) {
	l.log("F", msg)
}

func (l testLogger) Error(msg string) {
	l.log("E", msg)
}

func (l testLogger) Warn(msg string) {
	l.log("W", msg)
}

func (l testLogger) Info(msg string) {
	l.log("I", msg)
}

func (l testLogger) Infof(msg string, args ...interface{}) {
	l.log("I", fmt.Sprintf(msg, args...))
}

func (l testLogger) Debug(msg string) {
	l.log("D", msg)
}

func (l testLogger) Debugf(msg string, args ...interface{}) {
	l.log("D", fmt.Sprintf(msg, args...))
}

func (l testLogger) Fields() tchannel.LogFields {
	return l.fields
}

func (l testLogger) WithFields(fields ...tchannel.LogField) tchannel.Logger {
	existing := len(l.Fields())
	newFields := make(tchannel.LogFields, existing+len(fields))
	copy(newFields, l.Fields())
	copy(newFields[existing:], fields)
	return testLogger{l.t, newFields, l.w}
}

func (l testLogger) report() {
	if l.t.Failed() {
		l.w.withLock(func(w *bytes.Buffer) {
			l.t.Logf("Debug logs:\n%s", w.String())
		})
	}
}

// checkFilters returns whether the message can be ignored by the filters.
func (l errorLogger) checkFilters(msg string) bool {
	match := -1
	for i, filter := range l.v.Filters {
		if filter.Matches(msg, l.Fields()) {
			match = i
		}
	}

	if match == -1 {
		return false
	}

	matchCount := l.s.matchCount[match].Inc()
	return uint(matchCount) <= l.v.Filters[match].Count
}

func (l errorLogger) checkErr(prefix, msg string) {
	if l.checkFilters(msg) {
		return
	}

	l.t.Errorf("Unexpected log: %v: %s %v", prefix, msg, l.Logger.Fields())
}

func (l errorLogger) Fatal(msg string) {
	l.checkErr("[Fatal]", msg)
	l.Logger.Fatal(msg)
}

func (l errorLogger) Error(msg string) {
	l.checkErr("[Error]", msg)
	l.Logger.Error(msg)
}

func (l errorLogger) Warn(msg string) {
	l.checkErr("[Warn]", msg)
	l.Logger.Warn(msg)
}

func (l errorLogger) WithFields(fields ...tchannel.LogField) tchannel.Logger {
	return errorLogger{l.Logger.WithFields(fields...), l.t, l.v, l.s}
}
