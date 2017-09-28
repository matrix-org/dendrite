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

package tchannel

import (
	"bytes"
	"io"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeMessage(w io.Writer, msg message) error {
	f := NewFrame(MaxFramePayloadSize)
	if err := f.write(msg); err != nil {
		return err
	}
	return f.WriteOut(w)
}

func readFrame(r io.Reader) (*Frame, error) {
	f := NewFrame(MaxFramePayloadSize)
	return f, f.ReadIn(r)
}

func TestUnexpectedInitReq(t *testing.T) {
	tests := []struct {
		name          string
		initMsg       message
		expectedError errorMessage
	}{
		{
			name: "bad version",
			initMsg: &initReq{initMessage{id: 1, Version: 0x1, initParams: initParams{
				InitParamHostPort:    "0.0.0.0:0",
				InitParamProcessName: "test",
			}}},
			expectedError: errorMessage{
				id:      1,
				errCode: ErrCodeProtocol,
			},
		},
		{
			name: "missing InitParamHostPort",
			initMsg: &initReq{initMessage{id: 2, Version: CurrentProtocolVersion, initParams: initParams{
				InitParamProcessName: "test",
			}}},
			expectedError: errorMessage{
				id:      2,
				errCode: ErrCodeProtocol,
			},
		},
		{
			name: "missing InitParamProcessName",
			initMsg: &initReq{initMessage{id: 3, Version: CurrentProtocolVersion, initParams: initParams{
				InitParamHostPort: "0.0.0.0:0",
			}}},
			expectedError: errorMessage{
				id:      3,
				errCode: ErrCodeProtocol,
			},
		},
		{
			name: "unexpected message type",
			initMsg: &pingReq{
				id: 1,
			},
			expectedError: errorMessage{
				id:      1,
				errCode: ErrCodeProtocol,
			},
		},
	}

	for _, tt := range tests {
		ch, err := NewChannel("test", nil)
		require.NoError(t, err)
		defer ch.Close()
		require.NoError(t, ch.ListenAndServe("127.0.0.1:0"))
		hostPort := ch.PeerInfo().HostPort

		conn, err := net.Dial("tcp", hostPort)
		require.NoError(t, err)
		conn.SetReadDeadline(time.Now().Add(time.Second))

		if !assert.NoError(t, writeMessage(conn, tt.initMsg), "write to conn failed") {
			continue
		}

		f, err := readFrame(conn)
		if !assert.NoError(t, err, "read frame failed") {
			continue
		}
		assert.Equal(t, messageTypeError, f.Header.messageType)
		var errMsg errorMessage
		if !assert.NoError(t, f.read(&errMsg), "parse frame to errorMessage") {
			continue
		}
		assert.Equal(t, tt.expectedError.ID(), f.Header.ID, "test %v got bad ID", tt.name)
		assert.Equal(t, tt.expectedError.errCode, errMsg.errCode, "test %v got bad code", tt.name)
		assert.NoError(t, conn.Close(), "closing connection failed")
	}
}

func TestUnexpectedInitRes(t *testing.T) {
	validParams := initParams{
		InitParamHostPort:    "0.0.0.0:0",
		InitParamProcessName: "tchannel-go.test",
	}
	tests := []struct {
		msg    message
		errMsg string
	}{
		{
			msg: &initRes{initMessage{
				id:         1,
				Version:    CurrentProtocolVersion - 1,
				initParams: validParams,
			}},
			errMsg: "unsupported protocol version",
		},
		{
			msg: &initRes{initMessage{
				id:         1,
				Version:    CurrentProtocolVersion + 1,
				initParams: validParams,
			}},
			errMsg: "unsupported protocol version",
		},
		{
			msg: &initRes{initMessage{
				id:      1,
				Version: CurrentProtocolVersion,
			}},
			errMsg: "header host_port is required",
		},
		{
			msg: &initRes{initMessage{
				id:      1,
				Version: CurrentProtocolVersion,
				initParams: initParams{
					InitParamHostPort: "0.0.0.0:0",
				},
			}},
			errMsg: "header process_name is required",
		},
	}

	for _, tt := range tests {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err, "net.Listen failed")
		defer ln.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			ch, err := NewChannel("test", nil)
			require.NoError(t, err)
			defer ch.Close()

			ctx, cancel := NewContext(time.Second)
			defer cancel()
			_, err = ch.Peers().GetOrAdd(ln.Addr().String()).GetConnection(ctx)
			if !assert.Error(t, err, "Expected GetConnection to fail") {
				return
			}

			assert.Equal(t, ErrCodeProtocol, GetSystemErrorCode(err), "Unexpected error code, got error: %v", err)
			assert.Contains(t, err.Error(), tt.errMsg)
		}()

		conn, err := ln.Accept()
		require.NoError(t, err, "Failed to accept connection")

		// Read the frame and verify that it's an initReq.
		f, err := readFrame(conn)
		require.NoError(t, err, "read frame failed")
		if !assert.Equal(t, messageTypeInitReq, f.messageType(), "Expected first message to be initReq") {
			continue
		}

		// Write out the specified initRes wait for the channel to get an error.
		assert.NoError(t, writeMessage(conn, tt.msg), "write initRes failed")
		<-done
	}
}

func TestHandleInitReqNewVersion(t *testing.T) {
	ch, err := NewChannel("test", nil)
	require.NoError(t, err)
	defer ch.Close()
	require.NoError(t, ch.ListenAndServe("127.0.0.1:0"))
	hostPort := ch.PeerInfo().HostPort

	conn, err := net.Dial("tcp", hostPort)
	require.NoError(t, err)
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(time.Second))

	initMsg := &initReq{initMessage{id: 1, Version: CurrentProtocolVersion + 3, initParams: initParams{
		InitParamHostPort:    "0.0.0.0:0",
		InitParamProcessName: "test",
	}}}
	require.NoError(t, writeMessage(conn, initMsg), "write to conn failed")

	// Verify we get an initRes back with the current protocol version.
	f, err := readFrame(conn)
	require.NoError(t, err, "expected frame with init res")

	var msg initRes
	require.NoError(t, f.read(&msg), "could not read init res from frame")
	if assert.Equal(t, messageTypeInitRes, f.Header.messageType, "expected initRes, got %v", f.Header.messageType) {
		assert.Equal(t, initRes{
			initMessage: initMessage{
				Version: CurrentProtocolVersion,
				initParams: initParams{
					InitParamHostPort:                ch.PeerInfo().HostPort,
					InitParamProcessName:             ch.PeerInfo().ProcessName,
					InitParamTChannelLanguage:        "go",
					InitParamTChannelLanguageVersion: strings.TrimPrefix(runtime.Version(), "go"),
					InitParamTChannelVersion:         VersionInfo,
				},
			},
		}, msg, "unexpected init res")
	}
}

// TestHandleInitRes ensures that a Connection is ready to handle messages immediately
// after receiving an InitRes.
func TestHandleInitRes(t *testing.T) {
	l := newListener(t)
	listenerComplete := make(chan struct{})

	go func() {
		conn, err := l.Accept()
		require.NoError(t, err, "l.Accept failed")

		// The connection should be kept open until the test has completed running.
		defer conn.Close()
		defer func() { listenerComplete <- struct{}{} }()

		f, err := readFrame(conn)
		require.NoError(t, err, "readFrame failed")
		assert.Equal(t, messageTypeInitReq, f.Header.messageType, "expected initReq message")

		var msg initReq
		require.NoError(t, f.read(&msg), "read frame into initMsg failed")
		initRes := initRes{msg.initMessage}
		initRes.initMessage.id = f.Header.ID
		require.NoError(t, writeMessage(conn, &initRes), "write initRes failed")
		require.NoError(t, writeMessage(conn, &pingReq{noBodyMsg{}, 10}), "write pingReq failed")

		f, err = readFrame(conn)
		require.NoError(t, err, "readFrame failed")
		assert.Equal(t, messageTypePingRes, f.Header.messageType, "expected pingRes message")
	}()

	ch, err := NewChannel("test-svc", nil)
	require.NoError(t, err, "NewChannel failed")
	defer ch.Close()

	ctx, cancel := NewContext(time.Second)
	defer cancel()

	_, err = ch.Peers().GetOrAdd(l.Addr().String()).GetConnection(ctx)
	require.NoError(t, err, "GetConnection failed")

	<-listenerComplete
}

func TestInitReqGetsError(t *testing.T) {
	l := newListener(t)
	listenerComplete := make(chan struct{})
	connectionComplete := make(chan struct{})
	go func() {
		defer func() { listenerComplete <- struct{}{} }()
		conn, err := l.Accept()
		require.NoError(t, err, "l.Accept failed")
		defer conn.Close()

		f, err := readFrame(conn)
		require.NoError(t, err, "readFrame failed")
		assert.Equal(t, messageTypeInitReq, f.Header.messageType, "expected initReq message")
		err = writeMessage(conn, &errorMessage{
			id:      f.Header.ID,
			errCode: ErrCodeBadRequest,
			message: "invalid host:port",
		})
		assert.NoError(t, err, "Failed to write errorMessage")
		// Wait till GetConnection returns before closing the connection.
		<-connectionComplete
	}()

	logOut := &bytes.Buffer{}
	ch, err := NewChannel("test-svc", &ChannelOptions{Logger: NewLevelLogger(NewLogger(logOut), LogLevelWarn)})
	require.NoError(t, err, "NewClient failed")
	defer ch.Close()

	ctx, cancel := NewContext(time.Second)
	defer cancel()

	_, err = ch.Peers().GetOrAdd(l.Addr().String()).GetConnection(ctx)
	expectedErr := NewSystemError(ErrCodeBadRequest, "invalid host:port")
	assert.Equal(t, expectedErr, err, "Error mismatch")
	assert.Contains(t, logOut.String(),
		"[E] Failed during connection handshake.",
		"Message should be logged")
	assert.Contains(t, logOut.String(),
		"tchannel error ErrCodeBadRequest: invalid host:port",
		"Error should be logged")
	close(connectionComplete)

	<-listenerComplete
}

func newListener(t *testing.T) net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Listen failed")
	return l
}
