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

package benchmark

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/raw"
	"github.com/uber/tchannel-go/testutils"
)

const (
	_idOffset    = 4             /* size (2) + type (1) + reserved (1) */
	_idOffsetEnd = _idOffset + 4 /* length */
)

type frames struct {
	outgoing [][]byte
	incoming [][]byte
}

func (f frames) duplicate() frames {
	return frames{
		outgoing: deepCopyByteSlice(f.outgoing),
		incoming: deepCopyByteSlice(f.incoming),
	}
}

func deepCopyByteSlice(bs [][]byte) [][]byte {
	newBs := make([][]byte, len(bs))
	for i, b := range bs {
		newBs[i] = make([]byte, len(b))
		copy(newBs[i], b)
	}
	return newBs
}

func (f frames) writeInitReq(w io.Writer) error {
	_, err := w.Write(f.outgoing[0])
	return err
}

func (f frames) writeInitRes(w io.Writer) error {
	_, err := w.Write(f.incoming[0])
	return err
}

func (f frames) writeCallReq(id uint32, w io.Writer) (int, error) {
	frames := f.outgoing[1:]
	return f.writeMulti(id, w, frames)
}

func (f frames) writeCallRes(id uint32, w io.Writer) (int, error) {
	frames := f.incoming[1:]
	return f.writeMulti(id, w, frames)
}

func (f frames) writeMulti(id uint32, w io.Writer, frames [][]byte) (int, error) {
	written := 0
	for _, f := range frames {
		binary.BigEndian.PutUint32(f[_idOffset:_idOffsetEnd], id)
		if _, err := w.Write(f); err != nil {
			return written, err
		}
		written++
	}

	return written, nil
}

func getRawCallFrames(timeout time.Duration, svcName string, reqSize int) frames {
	var fs frames
	modifier := func(fromClient bool, f *tchannel.Frame) *tchannel.Frame {
		buf := &bytes.Buffer{}
		if err := f.WriteOut(buf); err != nil {
			panic(err)
		}

		if fromClient {
			fs.outgoing = append(fs.outgoing, buf.Bytes())
		} else {
			fs.incoming = append(fs.incoming, buf.Bytes())
		}

		return f
	}

	withNewServerClient(svcName, func(server, client *tchannel.Channel) {
		testutils.RegisterEcho(server, nil)

		relay, err := NewTCPFrameRelay([]string{server.PeerInfo().HostPort}, modifier)
		if err != nil {
			panic(err)
		}
		defer relay.Close()

		args := &raw.Args{
			Arg2: getRequestBytes(reqSize),
			Arg3: getRequestBytes(reqSize),
		}

		ctx, cancel := tchannel.NewContext(timeout)
		defer cancel()

		if _, _, _, err := raw.Call(ctx, client, relay.HostPort(), svcName, "echo", args.Arg2, args.Arg3); err != nil {
			panic(err)
		}
	})

	return fs
}

func withNewServerClient(svcName string, f func(server, client *tchannel.Channel)) {
	opts := testutils.NewOpts().SetServiceName(svcName)
	server, err := testutils.NewServerChannel(opts)
	if err != nil {
		panic(err)
	}
	defer server.Close()

	client, err := testutils.NewClientChannel(opts)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	f(server, client)
}
