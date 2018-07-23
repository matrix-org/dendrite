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

type Tracers struct {
	cfg     *config.Dendrite
	closers []io.Closer
}

func NoopTracers() *Tracers {
	return &Tracers{}
}

func NewTracers(cfg *config.Dendrite) *Tracers {
	return &Tracers{
		cfg: cfg,
	}
}

func (t *Tracers) InitGlobalTracer(serviceName string) error {
	if t.cfg == nil {
		return nil
	}

	// Set up GlobalTracer
	closer, err := t.cfg.Tracing.Jaeger.InitGlobalTracer(
		serviceName,
		jaegerconfig.Logger(logrusLogger{logrus.StandardLogger()}),
		jaegerconfig.Metrics(jaegermetrics.NullFactory),
	)

	if err != nil {
		return err
	}

	t.closers = append(t.closers, closer)

	return nil
}

// SetupTracing configures the opentracing using the supplied configuration.
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

func (l logrusLogger) Error(msg string) {
	l.l.Error(msg)
}

func (l logrusLogger) Infof(msg string, args ...interface{}) {
	l.l.Infof(msg, args...)
}
