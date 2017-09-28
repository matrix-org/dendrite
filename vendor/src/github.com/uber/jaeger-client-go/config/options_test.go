// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"testing"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger-client-go"
)

func TestApplyOptions(t *testing.T) {
	metricsFactory := metrics.NewLocalFactory(0)
	observer := fakeObserver{}
	contribObserver := fakeContribObserver{}
	opts := applyOptions(
		Metrics(metricsFactory),
		Logger(jaeger.StdLogger),
		Observer(observer),
		ContribObserver(contribObserver),
		Gen128Bit(true),
		ZipkinSharedRPCSpan(true),
	)
	assert.Equal(t, jaeger.StdLogger, opts.logger)
	assert.Equal(t, metricsFactory, opts.metrics)
	assert.Equal(t, []jaeger.Observer{observer}, opts.observers)
	assert.Equal(t, []jaeger.ContribObserver{contribObserver}, opts.contribObservers)
	assert.True(t, opts.gen128Bit)
	assert.True(t, opts.zipkinSharedRPCSpan)
}

func TestTraceTagOption(t *testing.T) {
	c := Configuration{}
	tracer, closer, err := c.New("test-service", Tag("tag-key", "tag-value"))
	require.NoError(t, err)
	defer closer.Close()
	assert.Equal(t, opentracing.Tag{Key: "tag-key", Value: "tag-value"}, tracer.(*jaeger.Tracer).Tags()[0])
}

func TestApplyOptionsDefaults(t *testing.T) {
	opts := applyOptions()
	assert.Equal(t, jaeger.NullLogger, opts.logger)
	assert.Equal(t, metrics.NullFactory, opts.metrics)
}

type fakeObserver struct{}

func (o fakeObserver) OnStartSpan(operationName string, options opentracing.StartSpanOptions) jaeger.SpanObserver {
	return nil
}

type fakeContribObserver struct{}

func (o fakeContribObserver) OnStartSpan(span opentracing.Span, operationName string, options opentracing.StartSpanOptions) (jaeger.ContribSpanObserver, bool) {
	return nil, false
}
