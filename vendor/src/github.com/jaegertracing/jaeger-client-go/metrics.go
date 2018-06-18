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

package jaeger

import (
	"github.com/uber/jaeger-lib/metrics"
)

// Metrics is a container of all stats emitted by Jaeger tracer.
type Metrics struct {
	// Number of traces started by this tracer as sampled
	TracesStartedSampled metrics.Counter `metric:"traces" tags:"state=started,sampled=y"`

	// Number of traces started by this tracer as not sampled
	TracesStartedNotSampled metrics.Counter `metric:"traces" tags:"state=started,sampled=n"`

	// Number of externally started sampled traces this tracer joined
	TracesJoinedSampled metrics.Counter `metric:"traces" tags:"state=joined,sampled=y"`

	// Number of externally started not-sampled traces this tracer joined
	TracesJoinedNotSampled metrics.Counter `metric:"traces" tags:"state=joined,sampled=n"`

	// Number of sampled spans started by this tracer
	SpansStarted metrics.Counter `metric:"spans" tags:"group=lifecycle,state=started"`

	// Number of sampled spans finished by this tracer
	SpansFinished metrics.Counter `metric:"spans" tags:"group=lifecycle,state=finished"`

	// Number of sampled spans started by this tracer
	SpansSampled metrics.Counter `metric:"spans" tags:"group=sampling,sampled=y"`

	// Number of not-sampled spans started by this tracer
	SpansNotSampled metrics.Counter `metric:"spans" tags:"group=sampling,sampled=n"`

	// Number of errors decoding tracing context
	DecodingErrors metrics.Counter `metric:"decoding-errors"`

	// Number of spans successfully reported
	ReporterSuccess metrics.Counter `metric:"reporter-spans" tags:"state=success"`

	// Number of spans in failed attempts to report
	ReporterFailure metrics.Counter `metric:"reporter-spans" tags:"state=failure"`

	// Number of spans dropped due to internal queue overflow
	ReporterDropped metrics.Counter `metric:"reporter-spans" tags:"state=dropped"`

	// Current number of spans in the reporter queue
	ReporterQueueLength metrics.Gauge `metric:"reporter-queue"`

	// Number of times the Sampler succeeded to retrieve sampling strategy
	SamplerRetrieved metrics.Counter `metric:"sampler" tags:"state=retrieved"`

	// Number of times the Sampler succeeded to retrieve and update sampling strategy
	SamplerUpdated metrics.Counter `metric:"sampler" tags:"state=updated"`

	// Number of times the Sampler failed to update sampling strategy
	SamplerUpdateFailure metrics.Counter `metric:"sampler" tags:"state=failure,phase=updating"`

	// Number of times the Sampler failed to retrieve sampling strategy
	SamplerQueryFailure metrics.Counter `metric:"sampler" tags:"state=failure,phase=query"`

	// Number of times baggage was successfully written or updated on spans.
	BaggageUpdateSuccess metrics.Counter `metric:"baggage-update" tags:"result=ok"`

	// Number of times baggage failed to write or update on spans.
	BaggageUpdateFailure metrics.Counter `metric:"baggage-update" tags:"result=err"`

	// Number of times baggage was truncated as per baggage restrictions.
	BaggageTruncate metrics.Counter `metric:"baggage-truncate"`

	// Number of times baggage restrictions were successfully updated.
	BaggageRestrictionsUpdateSuccess metrics.Counter `metric:"baggage-restrictions-update" tags:"result=ok"`

	// Number of times baggage restrictions failed to update.
	BaggageRestrictionsUpdateFailure metrics.Counter `metric:"baggage-restrictions-update" tags:"result=err"`
}

// NewMetrics creates a new Metrics struct and initializes it.
func NewMetrics(factory metrics.Factory, globalTags map[string]string) *Metrics {
	m := &Metrics{}
	metrics.Init(m, factory.Namespace("jaeger", nil), globalTags)
	return m
}

// NewNullMetrics creates a new Metrics struct that won't report any metrics.
func NewNullMetrics() *Metrics {
	return NewMetrics(metrics.NullFactory, nil)
}
