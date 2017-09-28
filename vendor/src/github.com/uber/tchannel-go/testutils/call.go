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

package testutils

import (
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/relay"
)

// FakeIncomingCall implements IncomingCall interface.
// Note: the F suffix for the fields is to clash with the method name.
type FakeIncomingCall struct {
	// CallerNameF is the calling service's name.
	CallerNameF string

	// ShardKeyF is the intended destination for this call.
	ShardKeyF string

	// RemotePeerF is the calling service's peer info.
	RemotePeerF tchannel.PeerInfo

	// LocalPeerF is the local service's peer info.
	LocalPeerF tchannel.LocalPeerInfo

	// RoutingKeyF is the routing key.
	RoutingKeyF string

	// RoutingDelegateF is the routing delegate.
	RoutingDelegateF string
}

// CallerName returns the caller name as specified in the fake call.
func (f *FakeIncomingCall) CallerName() string {
	return f.CallerNameF
}

// ShardKey returns the shard key as specified in the fake call.
func (f *FakeIncomingCall) ShardKey() string {
	return f.ShardKeyF
}

// RoutingKey returns the routing delegate as specified in the fake call.
func (f *FakeIncomingCall) RoutingKey() string {
	return f.RoutingKeyF
}

// RoutingDelegate returns the routing delegate as specified in the fake call.
func (f *FakeIncomingCall) RoutingDelegate() string {
	return f.RoutingDelegateF
}

// LocalPeer returns the local peer information for this call.
func (f *FakeIncomingCall) LocalPeer() tchannel.LocalPeerInfo {
	return f.LocalPeerF
}

// RemotePeer returns the remote peer information for this call.
func (f *FakeIncomingCall) RemotePeer() tchannel.PeerInfo {
	return f.RemotePeerF
}

// CallOptions returns the incoming call options suitable for proxying a request.
func (f *FakeIncomingCall) CallOptions() *tchannel.CallOptions {
	return &tchannel.CallOptions{
		ShardKey:        f.ShardKey(),
		RoutingKey:      f.RoutingKey(),
		RoutingDelegate: f.RoutingDelegate(),
	}
}

// NewIncomingCall creates an incoming call for tests.
func NewIncomingCall(callerName string) tchannel.IncomingCall {
	return &FakeIncomingCall{CallerNameF: callerName}
}

// FakeCallFrame is a stub implementation of the CallFrame interface.
type FakeCallFrame struct {
	ServiceF, MethodF, CallerF, RoutingKeyF, RoutingDelegateF string
}

var _ relay.CallFrame = FakeCallFrame{}

// Service returns the service name field.
func (f FakeCallFrame) Service() []byte {
	return []byte(f.ServiceF)
}

// Method returns the method field.
func (f FakeCallFrame) Method() []byte {
	return []byte(f.MethodF)
}

// Caller returns the caller field.
func (f FakeCallFrame) Caller() []byte {
	return []byte(f.CallerF)
}

// RoutingKey returns the routing delegate field.
func (f FakeCallFrame) RoutingKey() []byte {
	return []byte(f.RoutingKeyF)
}

// RoutingDelegate returns the routing delegate field.
func (f FakeCallFrame) RoutingDelegate() []byte {
	return []byte(f.RoutingDelegateF)
}
