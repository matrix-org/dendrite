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

package hyperbahn

// Event describes different events that Client can trigger.
type Event int

const (
	// UnknownEvent should never be used.
	UnknownEvent Event = iota
	// SendAdvertise is triggered when the Hyperbahn client tries to advertise.
	SendAdvertise
	// Advertised is triggered when the initial advertisement for a service is successful.
	Advertised
	// Readvertised is triggered on periodic advertisements.
	Readvertised
)

//go:generate stringer -type=Event

// Handler is the interface for handling Hyperbahn events and errors.
type Handler interface {
	// On is called when events are triggered.
	On(event Event)
	// OnError is called when an error is detected.
	OnError(err error)
}

// nullHandler is the default Handler if nil is passed, so handlers can always be called.
type nullHandler struct{}

func (nullHandler) On(event Event)    {}
func (nullHandler) OnError(err error) {}
