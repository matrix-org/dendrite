// Copyright (c) 2016 Uber Technologies, Inc.
//
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

package crossdock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crossdock/crossdock-go/assert"
	"github.com/crossdock/crossdock-go/require"
)

func TestHandler(t *testing.T) {
	tests := []struct {
		failOnUnknown bool
		status        string
	}{
		{false, "skipped"},
		{true, "failed"},
	}

	for _, test := range tests {
		testHandler(t, test.failOnUnknown, test.status)
	}
}

func testHandler(t *testing.T, failOnUnknown bool, status string) {
	behaviors := Behaviors{
		"b1": func(t T) {
			t.Successf("ok")
		},
	}

	server := httptest.NewServer(Handler(behaviors, failOnUnknown))
	defer server.Close()

	runTestCase(t, server.URL, "b1", map[string]string{
		"status": "passed",
		"output": "ok",
	})

	runTestCase(t, server.URL, "b2", map[string]string{
		"status": status,
		"output": `unknown behavior "b2"`,
	})
}

func runTestCase(t *testing.T, url string, behavior string, expectation map[string]string) {
	res, err := http.Get(fmt.Sprintf("%s/?behavior=%s", url, behavior))
	require.NoError(t, err)

	defer res.Body.Close()

	var answer []map[string]string
	err = json.NewDecoder(res.Body).Decode(&answer)
	require.NoError(t, err)

	assert.Equal(t, []map[string]string{expectation}, answer)
}
