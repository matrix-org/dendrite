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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	. "github.com/uber/tchannel-go"

	"github.com/uber/tchannel-go/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

const (
	streamRequestError = byte(255)
	streamRequestClose = byte(254)
)

func makeRepeatedBytes(n byte) []byte {
	data := make([]byte, int(n))
	for i := byte(0); i < n; i++ {
		data[i] = n
	}
	return data
}

func writeFlushBytes(w ArgWriter, bs []byte) error {
	if _, err := w.Write(bs); err != nil {
		return err
	}
	return w.Flush()
}

type streamHelper struct {
	t *testing.T
}

// startCall starts a call to echoStream and returns the arg3 reader and writer.
func (h streamHelper) startCall(ctx context.Context, ch *Channel, hostPort, serviceName string) (ArgWriter, ArgReader) {
	call, err := ch.BeginCall(ctx, hostPort, serviceName, "echoStream", nil)
	require.NoError(h.t, err, "BeginCall to echoStream failed")

	// Write empty headers
	require.NoError(h.t, NewArgWriter(call.Arg2Writer()).Write(nil), "Write empty headers failed")

	// Flush arg3 to force the call to start without any arg3.
	writer, err := call.Arg3Writer()
	require.NoError(h.t, err, "Arg3Writer failed")
	require.NoError(h.t, writer.Flush(), "Arg3Writer flush failed")

	// Read empty Headers
	response := call.Response()
	var arg2 []byte
	require.NoError(h.t, NewArgReader(response.Arg2Reader()).Read(&arg2), "Read headers failed")
	require.False(h.t, response.ApplicationError(), "echoStream failed due to application error")

	reader, err := response.Arg3Reader()
	require.NoError(h.t, err, "Arg3Reader failed")

	return writer, reader
}

// streamPartialHandler returns a streaming handler that has the following contract:
// read a byte, write N bytes where N = the byte that was read.
// The results are be written as soon as the byte is read.
func streamPartialHandler(t *testing.T, reportErrors bool) HandlerFunc {
	return func(ctx context.Context, call *InboundCall) {
		response := call.Response()
		onError := func(err error) {
			if reportErrors {
				t.Errorf("Handler error: %v", err)
			}
			response.SendSystemError(fmt.Errorf("failed to read arg2"))
		}

		var arg2 []byte
		if err := NewArgReader(call.Arg2Reader()).Read(&arg2); err != nil {
			onError(fmt.Errorf("failed to read arg2"))
			return
		}

		if err := NewArgWriter(response.Arg2Writer()).Write(nil); err != nil {
			onError(fmt.Errorf(""))
			return
		}

		argReader, err := call.Arg3Reader()
		if err != nil {
			onError(fmt.Errorf("failed to read arg3"))
			return
		}

		argWriter, err := response.Arg3Writer()
		if err != nil {
			onError(fmt.Errorf("arg3 writer failed"))
			return
		}

		// Flush arg3 which will force a frame with just arg2 to be sent.
		// The test reads arg2 before arg3 has been sent.
		if err := argWriter.Flush(); err != nil {
			onError(fmt.Errorf("arg3 flush failed"))
			return
		}

		arg3 := make([]byte, 1)
		for {
			n, err := argReader.Read(arg3)
			if err == io.EOF {
				break
			}
			if n == 0 && err == nil {
				err = fmt.Errorf("read 0 bytes")
			}
			if err != nil {
				onError(fmt.Errorf("arg3 Read failed: %v", err))
				return
			}

			// Magic number to cause a failure
			if arg3[0] == streamRequestError {
				// Make sure that the reader is closed.
				if err := argReader.Close(); err != nil {
					onError(fmt.Errorf("request error failed to close argReader: %v", err))
					return
				}

				response.SendSystemError(errors.New("intentional failure"))
				return
			}
			if arg3[0] == streamRequestClose {
				if err := argWriter.Close(); err != nil {
					onError(err)
				}
				return
			}

			// Write the number of bytes as specified by arg3[0]
			if _, err := argWriter.Write(makeRepeatedBytes(arg3[0])); err != nil {
				onError(fmt.Errorf("argWriter Write failed: %v", err))
				return
			}
			if err := argWriter.Flush(); err != nil {
				onError(fmt.Errorf("argWriter flush failed: %v", err))
				return
			}
		}

		if err := argReader.Close(); err != nil {
			onError(fmt.Errorf("argReader Close failed: %v", err))
			return
		}

		if err := argWriter.Close(); err != nil {
			onError(fmt.Errorf("arg3writer Close failed: %v", err))
			return
		}
	}
}

func testStreamArg(t *testing.T, f func(argWriter ArgWriter, argReader ArgReader)) {
	defer testutils.SetTimeout(t, 2*time.Second)()
	ctx, cancel := NewContext(time.Second)
	defer cancel()

	helper := streamHelper{t}
	WithVerifiedServer(t, nil, func(ch *Channel, hostPort string) {
		ch.Register(streamPartialHandler(t, true /* report errors */), "echoStream")

		argWriter, argReader := helper.startCall(ctx, ch, hostPort, ch.ServiceName())
		verifyBytes := func(n byte) {
			require.NoError(t, writeFlushBytes(argWriter, []byte{n}), "arg3 write failed")

			arg3 := make([]byte, int(n))
			_, err := io.ReadFull(argReader, arg3)
			require.NoError(t, err, "arg3 read failed")

			assert.Equal(t, makeRepeatedBytes(n), arg3, "arg3 result mismatch")
		}

		verifyBytes(0)
		verifyBytes(5)
		verifyBytes(100)
		verifyBytes(1)

		f(argWriter, argReader)
	})
}

func TestStreamPartialArg(t *testing.T) {
	testStreamArg(t, func(argWriter ArgWriter, argReader ArgReader) {
		require.NoError(t, argWriter.Close(), "arg3 close failed")

		// Once closed, we expect the reader to return EOF
		n, err := io.Copy(ioutil.Discard, argReader)
		assert.Equal(t, int64(0), n, "arg2 reader expected to EOF after arg3 writer is closed")
		assert.NoError(t, err, "Copy should not fail")
		assert.NoError(t, argReader.Close(), "close arg reader failed")
	})
}

func TestStreamSendError(t *testing.T) {
	testStreamArg(t, func(argWriter ArgWriter, argReader ArgReader) {
		// Send the magic number to request an error.
		_, err := argWriter.Write([]byte{streamRequestError})
		require.NoError(t, err, "arg3 write failed")
		require.NoError(t, argWriter.Close(), "arg3 close failed")

		// Now we expect an error on our next read.
		_, err = ioutil.ReadAll(argReader)
		assert.Error(t, err, "ReadAll should fail")
		assert.True(t, strings.Contains(err.Error(), "intentional failure"), "err %v unexpected", err)
	})
}

func TestStreamCancelled(t *testing.T) {
	// Since the cancel message is unimplemented, the relay does not know that the
	// call was cancelled, andwill block closing till the timeout.
	opts := testutils.NewOpts().NoRelay()
	testutils.WithTestServer(t, opts, func(ts *testutils.TestServer) {
		ts.Register(streamPartialHandler(t, false /* report errors */), "echoStream")

		ctx, cancel := NewContext(testutils.Timeout(time.Second))
		defer cancel()

		helper := streamHelper{t}
		ch := ts.NewClient(nil)
		cancelContext := make(chan struct{})

		arg3Writer, arg3Reader := helper.startCall(ctx, ch, ts.HostPort(), ts.ServiceName())
		go func() {
			for i := 0; i < 10; i++ {
				assert.NoError(t, writeFlushBytes(arg3Writer, []byte{1}), "Write failed")
			}

			// Our reads and writes should fail now.
			<-cancelContext
			cancel()

			_, err := arg3Writer.Write([]byte{1})
			// The write will succeed since it's buffered.
			assert.NoError(t, err, "Write after fail should be buffered")
			assert.Error(t, arg3Writer.Flush(), "writer.Flush should fail after cancel")
			assert.Error(t, arg3Writer.Close(), "writer.Close should fail after cancel")
		}()

		for i := 0; i < 10; i++ {
			arg3 := make([]byte, 1)
			n, err := arg3Reader.Read(arg3)
			assert.Equal(t, 1, n, "Read did not correct number of bytes")
			assert.NoError(t, err, "Read failed")
		}

		close(cancelContext)

		n, err := io.Copy(ioutil.Discard, arg3Reader)
		assert.EqualValues(t, 0, n, "Read should not read any bytes after cancel")
		assert.Error(t, err, "Read should fail after cancel")
		assert.Error(t, arg3Reader.Close(), "reader.Close should fail after cancel")
	})
}

func TestResponseClosedBeforeRequest(t *testing.T) {
	testutils.WithTestServer(t, nil, func(ts *testutils.TestServer) {
		ts.Register(streamPartialHandler(t, false /* report errors */), "echoStream")

		ctx, cancel := NewContext(testutils.Timeout(time.Second))
		defer cancel()

		helper := streamHelper{t}
		ch := ts.NewClient(nil)
		responseClosed := make(chan struct{})
		writerDone := make(chan struct{})

		arg3Writer, arg3Reader := helper.startCall(ctx, ch, ts.HostPort(), ts.Server().ServiceName())
		go func() {
			defer close(writerDone)

			for i := 0; i < 10; i++ {
				assert.NoError(t, writeFlushBytes(arg3Writer, []byte{1}), "Write failed")
			}

			// Ignore the error of writeFlushBytes here since once we flush, the
			// remote side could receive and close the response before we've created
			// a new fragment (see fragmentingWriter.Flush). This could result
			// in the Flush returning a "mex is already shutdown" error.
			writeFlushBytes(arg3Writer, []byte{streamRequestClose})

			// Wait until our reader gets the EOF.
			<-responseClosed

			// Now our writes should fail, since the stream is shutdown
			err := writeFlushBytes(arg3Writer, []byte{1})
			if assert.Error(t, err, "Req write should fail since response stream ended") {
				assert.Contains(t, err.Error(), "mex has been shutdown")
			}
		}()

		for i := 0; i < 10; i++ {
			arg3 := make([]byte, 1)
			n, err := arg3Reader.Read(arg3)
			assert.Equal(t, 1, n, "Read did not correct number of bytes")
			assert.NoError(t, err, "Read failed")
		}

		eofBuf := make([]byte, 1)
		_, err := arg3Reader.Read(eofBuf)
		assert.Equal(t, io.EOF, err, "Response should EOF after request close")
		assert.NoError(t, arg3Reader.Close(), "Close should succeed")
		close(responseClosed)
		<-writerDone
	})
}
