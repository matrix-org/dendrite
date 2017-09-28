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
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/prashantv/protectmem"
)

type RecordingFramePool struct {
	sync.Mutex

	allocations map[*Frame]string
	badRelease  []string
}

type protectMemAllocs struct {
	frameAlloc  *protectmem.Allocation
	bufferAlloc *protectmem.Allocation
}

type ProtectMemFramePool struct {
	sync.Mutex

	allocations map[*Frame]protectMemAllocs
}

func NewRecordingFramePool() *RecordingFramePool {
	return &RecordingFramePool{
		allocations: make(map[*Frame]string),
	}
}

func recordStack() string {
	buf := make([]byte, 4096)
	runtime.Stack(buf, false)
	return string(buf)
}

func (p *RecordingFramePool) Get() *Frame {
	p.Lock()
	defer p.Unlock()

	frame := NewFrame(MaxFramePayloadSize)
	p.allocations[frame] = recordStack()
	return frame
}

func zeroOut(bs []byte) {
	for i := range bs {
		bs[i] = 0
	}
}

func (p *RecordingFramePool) Release(f *Frame) {
	// Make sure the payload is not used after this point by clearing the frame.
	zeroOut(f.Payload)
	f.Payload = nil
	zeroOut(f.buffer)
	f.buffer = nil
	zeroOut(f.headerBuffer)
	f.headerBuffer = nil
	f.Header = FrameHeader{}

	p.Lock()
	defer p.Unlock()

	if _, ok := p.allocations[f]; !ok {
		p.badRelease = append(p.badRelease, "bad Release at "+recordStack())
		return
	}

	delete(p.allocations, f)
}

func (p *RecordingFramePool) CheckEmpty() (int, string) {
	p.Lock()
	defer p.Unlock()

	var badCalls []string
	badCalls = append(badCalls, p.badRelease...)
	for f, s := range p.allocations {
		badCalls = append(badCalls, fmt.Sprintf("frame %p: %v not released, get from: %v", f, f.Header, s))
	}
	return len(p.allocations), strings.Join(badCalls, "\n")
}

// NewProtectMemFramePool creates a frame pool that ensures that released frames
// are not reused by removing all access to a frame once it's been released.
func NewProtectMemFramePool() FramePool {
	return &ProtectMemFramePool{
		allocations: make(map[*Frame]protectMemAllocs),
	}
}
func (p *ProtectMemFramePool) Get() *Frame {
	frameAlloc := protectmem.Allocate(unsafe.Sizeof(Frame{}))
	f := (*Frame)(frameAlloc.Ptr())

	bufferAlloc := protectmem.AllocateSlice(&f.buffer, MaxFramePayloadSize)
	f.buffer = f.buffer[:MaxFramePayloadSize]
	f.Payload = f.buffer[FrameHeaderSize:]
	f.headerBuffer = f.buffer[:FrameHeaderSize]

	p.Lock()
	p.allocations[f] = protectMemAllocs{
		frameAlloc:  frameAlloc,
		bufferAlloc: bufferAlloc,
	}
	p.Unlock()

	return f
}

func (p *ProtectMemFramePool) Release(f *Frame) {
	p.Lock()
	allocs, ok := p.allocations[f]
	delete(p.allocations, f)
	p.Unlock()

	if !ok {
		panic(fmt.Errorf("released frame that was not allocated by pool: %v", f.Header))
	}

	allocs.bufferAlloc.Protect(protectmem.None)
	allocs.frameAlloc.Protect(protectmem.None)
}
