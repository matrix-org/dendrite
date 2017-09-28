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

package thrift

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/testutils"
	"github.com/uber/tchannel-go/thrift/gen-go/meta"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
)

func TestDefaultHealth(t *testing.T) {
	withMetaSetup(t, func(ctx Context, c tchanMeta, server *Server) {
		ret, err := c.Health(ctx)
		if assert.NoError(t, err, "Health endpoint failed") {
			assert.True(t, ret.Ok, "Health status mismatch")
			assert.Nil(t, ret.Message, "Health message mismatch")
		}
	})
}

func TestThriftIDL(t *testing.T) {
	withMetaSetup(t, func(ctx Context, c tchanMeta, server *Server) {
		_, err := c.ThriftIDL(ctx)
		assert.Error(t, err, "Health endpoint failed")
		assert.Contains(t, err.Error(), "unimplemented")
	})
}

func TestVersionInfo(t *testing.T) {
	withMetaSetup(t, func(ctx Context, c tchanMeta, server *Server) {
		ret, err := c.VersionInfo(ctx)
		if assert.NoError(t, err, "VersionInfo endpoint failed") {
			expected := &meta.VersionInfo{
				Language:        "go",
				LanguageVersion: strings.TrimPrefix(runtime.Version(), "go"),
				Version:         tchannel.VersionInfo,
			}
			assert.Equal(t, expected, ret, "Unexpected version info")
		}
	})
}

func customHealthEmpty(ctx Context) (bool, string) {
	return false, ""
}

func TestCustomHealthEmpty(t *testing.T) {
	withMetaSetup(t, func(ctx Context, c tchanMeta, server *Server) {
		server.RegisterHealthHandler(customHealthEmpty)
		ret, err := c.Health(ctx)
		if assert.NoError(t, err, "Health endpoint failed") {
			assert.False(t, ret.Ok, "Health status mismatch")
			assert.Nil(t, ret.Message, "Health message mismatch")
		}
	})
}

func customHealthNoEmpty(ctx Context) (bool, string) {
	return false, "from me"
}

func TestCustomHealthNoEmpty(t *testing.T) {
	withMetaSetup(t, func(ctx Context, c tchanMeta, server *Server) {
		server.RegisterHealthHandler(customHealthNoEmpty)
		ret, err := c.Health(ctx)
		if assert.NoError(t, err, "Health endpoint failed") {
			assert.False(t, ret.Ok, "Health status mismatch")
			assert.Equal(t, ret.Message, thrift.StringPtr("from me"), "Health message mismatch")
		}
	})
}

func withMetaSetup(t *testing.T, f func(ctx Context, c tchanMeta, server *Server)) {
	ctx, cancel := NewContext(time.Second * 10)
	defer cancel()

	// Start server
	tchan, server := setupMetaServer(t)
	defer tchan.Close()

	// Get client1
	c := getMetaClient(t, tchan.PeerInfo().HostPort)
	f(ctx, c, server)
}

func setupMetaServer(t *testing.T) (*tchannel.Channel, *Server) {
	tchan := testutils.NewServer(t, testutils.NewOpts().SetServiceName("meta"))
	server := NewServer(tchan)
	return tchan, server
}

func getMetaClient(t *testing.T, dst string) tchanMeta {
	tchan := testutils.NewClient(t, nil)
	tchan.Peers().Add(dst)
	thriftClient := NewClient(tchan, "meta", nil)
	return newTChanMetaClient(thriftClient)
}
