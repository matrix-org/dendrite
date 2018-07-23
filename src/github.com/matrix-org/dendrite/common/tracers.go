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
	"io"

	"github.com/matrix-org/dendrite/common/config"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	jaegerconfig "github.com/uber/jaeger-client-go/config"
	jaegermetrics "github.com/uber/jaeger-lib/metrics"
)

// Tracers is a collection of Jaeger OpenTracing tracers. There is one tracer
// per Dendrite component, and each is kept track of with a single Tracers
// object. Upon shutdown, each tracer is closed by calling the Tracers' Close()
// method.
type Tracers struct {
	cfg     *config.Dendrite
	closers []io.Closer
}

// NoopTracers is a Tracers object that will never contain any tracers. Disables tracing.
func NoopTracers() *Tracers {
	return &Tracers{}
}

// NewTracers creates a new Tracers object with the given Dendrite config.
func NewTracers(cfg *config.Dendrite) *Tracers {
	return &Tracers{
		cfg: cfg,
	}
}

// SetupNewTracer creates a new tracer and adds it to those being kept track of
// on the linked Tracers object.
func (t *Tracers) SetupNewTracer(serviceName string) opentracing.Tracer {
	if t.cfg == nil {
		return opentracing.NoopTracer{}
	}

	tracer, closer, err := t.cfg.Tracing.Jaeger.New(
		serviceName,
		jaegerconfig.Logger(logrusLogger{logrus.StandardLogger()}),
		jaegerconfig.Metrics(jaegermetrics.NullFactory),
	)

	if err != nil {
		logrus.Panicf("Failed to create new tracer %s: %s", serviceName, err)
	}

	t.closers = append(t.closers, closer)

	return tracer
}

// Close will close all Jaeger OpenTracing tracers associated with the Tracers object.
func (t *Tracers) Close() error {
	for _, c := range t.closers {
		c.Close() // nolint: errcheck
	}

	return nil
}

// logrusLogger is a small wrapper that implements jaeger.Logger using logrus.
type logrusLogger struct {
	l *logrus.Logger
}

// Error prints an error message to the logger
func (l logrusLogger) Error(msg string) {
	l.l.Error(msg)
}

// Infof prints an info message with printf formatting to the logger
func (l logrusLogger) Infof(msg string, args ...interface{}) {
	l.l.Infof(msg, args...)
}
