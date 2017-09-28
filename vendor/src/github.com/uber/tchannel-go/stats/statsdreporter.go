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

package stats

import (
	"time"

	"github.com/cactus/go-statsd-client/statsd"
	"github.com/uber/tchannel-go"
)

const samplingRate = 1.0

// MetricKey is called to generate the statsd key for a given metric and tags.
var MetricKey = DefaultMetricPrefix

type statsdReporter struct {
	client statsd.Statter
}

// NewStatsdReporter returns a StatsReporter that reports to statsd on the given addr.
func NewStatsdReporter(addr, prefix string) (tchannel.StatsReporter, error) {
	client, err := statsd.NewBufferedClient(addr, prefix, time.Second, 0)
	if err != nil {
		return nil, err
	}

	return NewStatsdReporterClient(client), nil
}

// NewStatsdReporterClient returns a StatsReporter that reports stats to the given client.
func NewStatsdReporterClient(client statsd.Statter) tchannel.StatsReporter {
	return &statsdReporter{client}
}

func (r *statsdReporter) IncCounter(name string, tags map[string]string, value int64) {
	// TODO(prashant): Deal with errors in the client.
	r.client.Inc(MetricKey(name, tags), value, samplingRate)
}

func (r *statsdReporter) UpdateGauge(name string, tags map[string]string, value int64) {
	r.client.Gauge(MetricKey(name, tags), value, samplingRate)
}

func (r *statsdReporter) RecordTimer(name string, tags map[string]string, d time.Duration) {
	r.client.TimingDuration(MetricKey(name, tags), d, samplingRate)
}
