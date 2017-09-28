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

package pprof

import (
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/uber/tchannel-go"
	thttp "github.com/uber/tchannel-go/http"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPProfEndpoint(t *testing.T) {
	ch := testutils.NewServer(t, nil)
	Register(ch)

	ctx, cancel := tchannel.NewContext(time.Second)
	defer cancel()

	req, err := http.NewRequest("GET", "/debug/pprof/block?debug=1", nil)
	require.NoError(t, err, "NewRequest failed")

	call, err := ch.BeginCall(ctx, ch.PeerInfo().HostPort, ch.ServiceName(), "_pprof", nil)
	require.NoError(t, err, "BeginCall failed")
	require.NoError(t, err, thttp.WriteRequest(call, req), "thttp.WriteRequest failed")

	response, err := thttp.ReadResponse(call.Response())
	require.NoError(t, err, "ReadResponse failed")

	assert.Equal(t, http.StatusOK, response.StatusCode)
	body, err := ioutil.ReadAll(response.Body)
	if assert.NoError(t, err, "Read body failed") {
		assert.Contains(t, string(body), "contention", "Response does not contain expected string")
	}
}
