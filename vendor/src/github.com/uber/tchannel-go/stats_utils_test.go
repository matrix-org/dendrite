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

package tchannel_test

// This file contains test setup logic, and is named with a _test.go suffix to
// ensure it's only compiled with tests.

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type statsValue struct {
	// count is the counter value if this metric is a counter.
	count int64

	// timers is the list of timer values if this metrics is a timer.
	timers []time.Duration
}

type recordingStatsReporter struct {
	sync.Mutex

	// Values is a map from the metricName -> map[tagMapAsString]*statsValue
	Values map[string]map[string]*statsValue

	// Expected stores expected counter values.
	Expected *recordingStatsReporter
}

func newRecordingStatsReporter() *recordingStatsReporter {
	return &recordingStatsReporter{
		Values: make(map[string]map[string]*statsValue),
		Expected: &recordingStatsReporter{
			Values: make(map[string]map[string]*statsValue),
		},
	}
}

// keysMap returns the keys of the given map as a sorted list of strings.
// If the map is not of the type map[string]* then the function will panic.
func keysMap(m interface{}) []string {
	var keys []string
	mapKeys := reflect.ValueOf(m).MapKeys()
	for _, v := range mapKeys {
		keys = append(keys, v.Interface().(string))
	}
	sort.Strings(keys)
	return keys
}

// tagsToString converts a map of tags to a string that can be used as a map key.
func tagsToString(tags map[string]string) string {
	var vals []string
	for _, k := range keysMap(tags) {
		vals = append(vals, fmt.Sprintf("%v = %v", k, tags[k]))
	}
	return strings.Join(vals, ", ")
}

func (r *recordingStatsReporter) getStat(name string, tags map[string]string) *statsValue {
	r.Lock()
	defer r.Unlock()

	tagMap, ok := r.Values[name]
	if !ok {
		tagMap = make(map[string]*statsValue)
		r.Values[name] = tagMap
	}

	tagStr := tagsToString(tags)
	statVal, ok := tagMap[tagStr]
	if !ok {
		statVal = &statsValue{}
		tagMap[tagStr] = statVal
	}

	return statVal
}

func (r *recordingStatsReporter) IncCounter(name string, tags map[string]string, value int64) {
	statVal := r.getStat(name, tags)
	statVal.count += value
}

func (r *recordingStatsReporter) RecordTimer(name string, tags map[string]string, d time.Duration) {
	statVal := r.getStat(name, tags)
	statVal.timers = append(statVal.timers, d)
}

func (r *recordingStatsReporter) Reset() {
	newReporter := newRecordingStatsReporter()
	r.Values = newReporter.Values
	r.Expected = newReporter.Expected
}

func (r *recordingStatsReporter) Validate(t *testing.T) {
	r.Lock()
	defer r.Unlock()

	assert.Equal(t, keysMap(r.Expected.Values), keysMap(r.Values),
		"Metric keys are different")
	for counterKey, counter := range r.Values {
		expectedCounter, ok := r.Expected.Values[counterKey]
		if !ok {
			continue
		}

		assert.Equal(t, keysMap(expectedCounter), keysMap(counter),
			"Metric %v has different reported tags", counterKey)
		for tags, stat := range counter {
			expectedStat, ok := expectedCounter[tags]
			if !ok {
				continue
			}

			assert.Equal(t, expectedStat, stat,
				"Metric %v with tags %v has mismatched value", counterKey, tags)
		}
	}
}

func (r *recordingStatsReporter) UpdateGauge(name string, tags map[string]string, value int64) {}
