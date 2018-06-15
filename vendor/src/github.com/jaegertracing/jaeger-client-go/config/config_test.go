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
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/testutils"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/log"
)

func TestNewSamplerConst(t *testing.T) {
	constTests := []struct {
		param    float64
		decision bool
	}{{1, true}, {0, false}}
	for _, tst := range constTests {
		cfg := &SamplerConfig{Type: jaeger.SamplerTypeConst, Param: tst.param}
		s, err := cfg.NewSampler("x", nil)
		require.NoError(t, err)
		s1, ok := s.(*jaeger.ConstSampler)
		require.True(t, ok, "converted to constSampler")
		require.Equal(t, tst.decision, s1.Decision, "decision")
	}
}

func TestNewSamplerProbabilistic(t *testing.T) {
	constTests := []struct {
		param float64
		error bool
	}{{1.5, true}, {0.5, false}}
	for _, tst := range constTests {
		cfg := &SamplerConfig{Type: jaeger.SamplerTypeProbabilistic, Param: tst.param}
		s, err := cfg.NewSampler("x", nil)
		if tst.error {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			_, ok := s.(*jaeger.ProbabilisticSampler)
			require.True(t, ok, "converted to ProbabilisticSampler")
		}
	}
}

func TestDefaultSampler(t *testing.T) {
	cfg := Configuration{
		Sampler: &SamplerConfig{Type: "InvalidType"},
	}
	_, _, err := cfg.New("testService")
	require.Error(t, err)
}

func TestInvalidSamplerType(t *testing.T) {
	cfg := &SamplerConfig{MaxOperations: 10}
	s, err := cfg.NewSampler("x", jaeger.NewNullMetrics())
	require.NoError(t, err)
	rcs, ok := s.(*jaeger.RemotelyControlledSampler)
	require.True(t, ok, "converted to RemotelyControlledSampler")
	rcs.Close()
}

func TestDefaultConfig(t *testing.T) {
	cfg := Configuration{}
	_, _, err := cfg.New("", Metrics(metrics.NullFactory), Logger(log.NullLogger))
	require.EqualError(t, err, "no service name provided")

	_, closer, err := cfg.New("testService")
	defer closer.Close()
	require.NoError(t, err)
}

func TestDisabledFlag(t *testing.T) {
	cfg := Configuration{Disabled: true}
	_, closer, err := cfg.New("testService")
	defer closer.Close()
	require.NoError(t, err)
}

func TestNewReporterError(t *testing.T) {
	cfg := Configuration{
		Reporter: &ReporterConfig{LocalAgentHostPort: "bad_local_agent"},
	}
	_, _, err := cfg.New("testService")
	require.Error(t, err)
}

func TestInitGlobalTracer(t *testing.T) {
	// Save the existing GlobalTracer and replace after finishing function
	prevTracer := opentracing.GlobalTracer()
	defer opentracing.InitGlobalTracer(prevTracer)
	noopTracer := opentracing.NoopTracer{}

	tests := []struct {
		cfg           Configuration
		shouldErr     bool
		tracerChanged bool
	}{
		{
			cfg:           Configuration{Disabled: true},
			shouldErr:     false,
			tracerChanged: false,
		},
		{
			cfg:           Configuration{Sampler: &SamplerConfig{Type: "InvalidType"}},
			shouldErr:     true,
			tracerChanged: false,
		},
		{
			cfg: Configuration{
				Sampler: &SamplerConfig{
					Type: "remote",
					SamplingRefreshInterval: 1,
				},
			},
			shouldErr:     false,
			tracerChanged: true,
		},
		{
			cfg:           Configuration{},
			shouldErr:     false,
			tracerChanged: true,
		},
	}
	for _, test := range tests {
		opentracing.InitGlobalTracer(noopTracer)
		_, err := test.cfg.InitGlobalTracer("testService")
		if test.shouldErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		if test.tracerChanged {
			require.NotEqual(t, noopTracer, opentracing.GlobalTracer())
		} else {
			require.Equal(t, noopTracer, opentracing.GlobalTracer())
		}
	}
}

func TestConfigWithReporter(t *testing.T) {
	c := Configuration{
		Sampler: &SamplerConfig{
			Type:  "const",
			Param: 1,
		},
	}
	r := jaeger.NewInMemoryReporter()
	tracer, closer, err := c.New("test", Reporter(r))
	require.NoError(t, err)
	defer closer.Close()

	tracer.StartSpan("test").Finish()
	assert.Len(t, r.GetSpans(), 1)
}

func TestConfigWithRPCMetrics(t *testing.T) {
	metrics := metrics.NewLocalFactory(0)
	c := Configuration{
		Sampler: &SamplerConfig{
			Type:  "const",
			Param: 1,
		},
		RPCMetrics: true,
	}
	r := jaeger.NewInMemoryReporter()
	tracer, closer, err := c.New(
		"test",
		Reporter(r),
		Metrics(metrics),
		ContribObserver(fakeContribObserver{}),
	)
	require.NoError(t, err)
	defer closer.Close()

	tracer.StartSpan("test", ext.SpanKindRPCServer).Finish()

	testutils.AssertCounterMetrics(t, metrics,
		testutils.ExpectedMetric{
			Name:  "jaeger-rpc.requests",
			Tags:  map[string]string{"component": "jaeger", "endpoint": "test", "error": "false"},
			Value: 1,
		},
	)
}

func TestBaggageRestrictionsConfig(t *testing.T) {
	m := metrics.NewLocalFactory(0)
	c := Configuration{
		BaggageRestrictions: &BaggageRestrictionsConfig{
			HostPort:        "not:1929213",
			RefreshInterval: time.Minute,
		},
	}
	_, closer, err := c.New(
		"test",
		Metrics(m),
	)
	require.NoError(t, err)
	defer closer.Close()

	metricName := "jaeger.baggage-restrictions-update"
	metricTags := map[string]string{"result": "err"}
	key := metrics.GetKey(metricName, metricTags, "|", "=")
	for i := 0; i < 100; i++ {
		// wait until the async initialization call is complete
		counters, _ := m.Snapshot()
		if _, ok := counters[key]; ok {
			break
		}
		time.Sleep(time.Millisecond)
	}

	testutils.AssertCounterMetrics(t, m,
		testutils.ExpectedMetric{
			Name:  metricName,
			Tags:  metricTags,
			Value: 1,
		},
	)
}

func TestConfigWithGen128Bit(t *testing.T) {
	c := Configuration{
		Sampler: &SamplerConfig{
			Type:  "const",
			Param: 1,
		},
		RPCMetrics: true,
	}
	tracer, closer, err := c.New("test", Gen128Bit(true))
	require.NoError(t, err)
	defer closer.Close()

	span := tracer.StartSpan("test")
	defer span.Finish()
	traceID := span.Context().(jaeger.SpanContext).TraceID()
	require.True(t, traceID.High != 0)
	require.True(t, traceID.Low != 0)
}
