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
	"errors"
	"runtime"
	"strings"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift/gen-go/meta"
)

// HealthFunc is the interface for custom health endpoints.
// ok is whether the service health is OK, and message is optional additional information for the health result.
type HealthFunc func(ctx Context) (ok bool, message string)

// healthHandler implements the default health check enpoint.
type metaHandler struct {
	healthFn HealthFunc
}

// newMetaHandler return a new HealthHandler instance.
func newMetaHandler() *metaHandler {
	return &metaHandler{healthFn: defaultHealth}
}

// Health returns true as default Health endpoint.
func (h *metaHandler) Health(ctx Context) (*meta.HealthStatus, error) {
	ok, message := h.healthFn(ctx)
	if message == "" {
		return &meta.HealthStatus{Ok: ok}, nil
	}
	return &meta.HealthStatus{Ok: ok, Message: &message}, nil
}

func (h *metaHandler) ThriftIDL(ctx Context) (*meta.ThriftIDLs, error) {
	// TODO(prashant): Add thriftIDL to the generated code.
	return nil, errors.New("unimplemented")
}

func (h *metaHandler) VersionInfo(ctx Context) (*meta.VersionInfo, error) {
	return &meta.VersionInfo{
		Language:        "go",
		LanguageVersion: strings.TrimPrefix(runtime.Version(), "go"),
		Version:         tchannel.VersionInfo,
	}, nil
}

func defaultHealth(ctx Context) (bool, string) {
	return true, ""
}

// SetHandler sets customized handler for health endpoint.
func (h *metaHandler) setHandler(f HealthFunc) {
	h.healthFn = f
}
