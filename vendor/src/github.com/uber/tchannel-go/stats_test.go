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

import (
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func tagsForOutboundCall(serverCh *Channel, clientCh *Channel, method string) map[string]string {
	host, _ := os.Hostname()
	return map[string]string{
		"app":             clientCh.PeerInfo().ProcessName,
		"host":            host,
		"service":         clientCh.PeerInfo().ServiceName,
		"target-service":  serverCh.PeerInfo().ServiceName,
		"target-endpoint": method,
	}
}

func tagsForInboundCall(serverCh *Channel, clientCh *Channel, method string) map[string]string {
	host, _ := os.Hostname()
	return map[string]string{
		"app":             serverCh.PeerInfo().ProcessName,
		"host":            host,
		"service":         serverCh.PeerInfo().ServiceName,
		"calling-service": clientCh.PeerInfo().ServiceName,
		"endpoint":        method,
	}
}

func TestStatsCalls(t *testing.T) {
	defer testutils.SetTimeout(t, 2*time.Second)()

	tests := []struct {
		method  string
		wantErr bool
	}{
		{
			method: "echo",
		},
		{
			method:  "app-error",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		initialTime := time.Date(2015, 2, 1, 10, 10, 0, 0, time.UTC)
		clientNow, clientNowFn := testutils.NowStub(initialTime)
		serverNow, serverNowFn := testutils.NowStub(initialTime)
		clientNowFn(100 * time.Millisecond)
		serverNowFn(50 * time.Millisecond)

		clientStats := newRecordingStatsReporter()
		serverStats := newRecordingStatsReporter()
		serverOpts := testutils.NewOpts().
			SetStatsReporter(serverStats).
			SetTimeNow(serverNow)
		WithVerifiedServer(t, serverOpts, func(serverCh *Channel, hostPort string) {
			handler := raw.Wrap(newTestHandler(t))
			serverCh.Register(handler, "echo")
			serverCh.Register(handler, "app-error")

			ch := testutils.NewClient(t, testutils.NewOpts().
				SetStatsReporter(clientStats).
				SetTimeNow(clientNow))
			defer ch.Close()

			ctx, cancel := NewContext(time.Second * 5)
			defer cancel()

			_, _, resp, err := raw.Call(ctx, ch, hostPort, testutils.DefaultServerName, tt.method, nil, nil)
			require.NoError(t, err, "Call(%v) should fail", tt.method)
			assert.Equal(t, tt.wantErr, resp.ApplicationError(), "Call(%v) check application error")

			outboundTags := tagsForOutboundCall(serverCh, ch, tt.method)
			inboundTags := tagsForInboundCall(serverCh, ch, tt.method)

			clientStats.Expected.IncCounter("outbound.calls.send", outboundTags, 1)
			clientStats.Expected.RecordTimer("outbound.calls.per-attempt.latency", outboundTags, 100*time.Millisecond)
			clientStats.Expected.RecordTimer("outbound.calls.latency", outboundTags, 100*time.Millisecond)
			serverStats.Expected.IncCounter("inbound.calls.recvd", inboundTags, 1)
			serverStats.Expected.RecordTimer("inbound.calls.latency", inboundTags, 50*time.Millisecond)

			if tt.wantErr {
				clientStats.Expected.IncCounter("outbound.calls.per-attempt.app-errors", outboundTags, 1)
				clientStats.Expected.IncCounter("outbound.calls.app-errors", outboundTags, 1)
				serverStats.Expected.IncCounter("inbound.calls.app-errors", inboundTags, 1)
			} else {
				clientStats.Expected.IncCounter("outbound.calls.success", outboundTags, 1)
				serverStats.Expected.IncCounter("inbound.calls.success", inboundTags, 1)
			}
		})

		clientStats.Validate(t)
		serverStats.Validate(t)
	}
}

func TestStatsWithRetries(t *testing.T) {
	defer testutils.SetTimeout(t, 2*time.Second)()
	a := testutils.DurationArray

	initialTime := time.Date(2015, 2, 1, 10, 10, 0, 0, time.UTC)
	nowStub, nowFn := testutils.NowStub(initialTime)

	clientStats := newRecordingStatsReporter()
	ch := testutils.NewClient(t, testutils.NewOpts().
		SetStatsReporter(clientStats).
		SetTimeNow(nowStub))
	defer ch.Close()

	nowFn(10 * time.Millisecond)
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	// TODO why do we need this??
	opts := testutils.NewOpts().NoRelay()
	WithVerifiedServer(t, opts, func(serverCh *Channel, hostPort string) {
		respErr := make(chan error, 1)
		testutils.RegisterFunc(serverCh, "req", func(ctx context.Context, args *raw.Args) (*raw.Res, error) {
			return &raw.Res{Arg2: args.Arg2, Arg3: args.Arg3}, <-respErr
		})
		ch.Peers().Add(serverCh.PeerInfo().HostPort)

		// timeNow is called at:
		// RunWithRetry start, per-attempt start, per-attempt end.
		// Each attempt takes 2 * step.
		tests := []struct {
			expectErr           error
			numFailures         int
			numAttempts         int
			overallLatency      time.Duration
			perAttemptLatencies []time.Duration
		}{
			{
				numFailures:         0,
				numAttempts:         1,
				perAttemptLatencies: a(10 * time.Millisecond),
				overallLatency:      20 * time.Millisecond,
			},
			{
				numFailures:         1,
				numAttempts:         2,
				perAttemptLatencies: a(10*time.Millisecond, 10*time.Millisecond),
				overallLatency:      40 * time.Millisecond,
			},
			{
				numFailures:         4,
				numAttempts:         5,
				perAttemptLatencies: a(10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond),
				overallLatency:      100 * time.Millisecond,
			},
			{
				numFailures:         5,
				numAttempts:         5,
				expectErr:           ErrServerBusy,
				perAttemptLatencies: a(10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond, 10*time.Millisecond),
				overallLatency:      100 * time.Millisecond,
			},
		}

		for _, tt := range tests {
			clientStats.Reset()
			err := ch.RunWithRetry(ctx, func(ctx context.Context, rs *RequestState) error {
				if rs.Attempt > tt.numFailures {
					respErr <- nil
				} else {
					respErr <- ErrServerBusy
				}

				sc := ch.GetSubChannel(serverCh.ServiceName())
				_, err := raw.CallV2(ctx, sc, raw.CArgs{
					Method:      "req",
					CallOptions: &CallOptions{RequestState: rs},
				})
				return err
			})
			assert.Equal(t, tt.expectErr, err, "RunWithRetry unexpected error")

			outboundTags := tagsForOutboundCall(serverCh, ch, "req")
			if tt.expectErr == nil {
				clientStats.Expected.IncCounter("outbound.calls.success", outboundTags, 1)
			}
			clientStats.Expected.IncCounter("outbound.calls.send", outboundTags, int64(tt.numAttempts))
			for i, latency := range tt.perAttemptLatencies {
				clientStats.Expected.RecordTimer("outbound.calls.per-attempt.latency", outboundTags, latency)
				if i > 0 {
					tags := tagsForOutboundCall(serverCh, ch, "req")
					tags["retry-count"] = fmt.Sprint(i)
					clientStats.Expected.IncCounter("outbound.calls.retries", tags, 1)
				}
			}
			clientStats.Expected.RecordTimer("outbound.calls.latency", outboundTags, tt.overallLatency)
			clientStats.Validate(t)
		}
	})
}
