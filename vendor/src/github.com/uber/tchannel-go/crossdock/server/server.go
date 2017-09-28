// Copyright (c) 2016 Uber Technologies, Inc.
//
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
// Copyright (c) 2016 Uber Technologies, Inc.
//
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

package server

import (
	"strings"

	"github.com/opentracing/opentracing-go"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/crossdock/common"
	"github.com/uber/tchannel-go/crossdock/log"
)

// Server implements S2-S3 servers
type Server struct {
	HostPort string
	Tracer   opentracing.Tracer
	Ch       *tchannel.Channel
}

// Start starts the test server called by the Client and other upstream servers.
func (s *Server) Start() error {
	if s.HostPort == "" {
		s.HostPort = ":" + common.DefaultServerPort
	}
	channelOpts := &tchannel.ChannelOptions{
		Tracer: s.Tracer,
	}
	ch, err := tchannel.NewChannel(common.DefaultServiceName, channelOpts)
	if err != nil {
		return err
	}

	if err := ch.ListenAndServe(s.HostPort); err != nil {
		return err
	}
	s.HostPort = ch.PeerInfo().HostPort // override in case it was ":0"
	log.Printf("Started tchannel server at %s\n", s.HostPort)
	s.Ch = ch
	return nil
}

// Close stops the server
func (s *Server) Close() {
	s.Ch.Close()
}

// Port returns the actual port the server listens to
func (s *Server) Port() string {
	hostPortSplit := strings.Split(s.HostPort, ":")
	port := hostPortSplit[len(hostPortSplit)-1]
	return port
}
