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

package hyperbahn_test

import (
	"testing"

	. "github.com/uber/tchannel-go/hyperbahn"

	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/testutils/mockhyperbahn"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withSetup(t *testing.T, f func(mh *mockhyperbahn.Mock, client *Client)) {
	mh, err := mockhyperbahn.New()
	require.NoError(t, err, "mockhyperbahn.New failed")
	defer mh.Close()

	clientCh := testutils.NewClient(t, nil)
	client, err := NewClient(clientCh, mh.Configuration(), nil)
	require.NoError(t, err, "NewClient failed")
	defer clientCh.Close()

	f(mh, client)
}

func TestDiscoverSuccess(t *testing.T) {
	peers := []string{
		"127.0.0.1:8000",
		"127.0.0.2:8001",
		"1.2.4.8:21150",
		"10.254.254.1:21151",
	}

	withSetup(t, func(mh *mockhyperbahn.Mock, client *Client) {
		mh.SetDiscoverResult("test-discover", peers)

		gotPeers, err := client.Discover("test-discover")
		assert.NoError(t, err, "Discover failed")
		assert.Equal(t, peers, gotPeers)
	})
}

func TestDiscoverFails(t *testing.T) {
	withSetup(t, func(mh *mockhyperbahn.Mock, client *Client) {
		_, err := client.Discover("test-discover")
		assert.Error(t, err, "Discover should fail due to no discover result set in the mock")
	})
}
