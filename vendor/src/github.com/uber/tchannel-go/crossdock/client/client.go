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

package client

import (
	"fmt"
	"net"
	"net/http"

	"github.com/uber/tchannel-go/crossdock/common"

	"github.com/crossdock/crossdock-go"
)

// Client is a controller for the tests
type Client struct {
	ClientHostPort string
	ServerPort     string
	listener       net.Listener
	mux            *http.ServeMux
	Behaviors      crossdock.Behaviors
}

// Start begins a Crossdock client in the background.
func (c *Client) Start() error {
	if err := c.listen(); err != nil {
		return err
	}
	go func() {
		http.Serve(c.listener, c.mux)
	}()
	return nil
}

// Listen initializes the server
func (c *Client) listen() error {
	c.setDefaultPort(&c.ClientHostPort, ":"+common.DefaultClientPortHTTP)
	c.setDefaultPort(&c.ServerPort, common.DefaultServerPort)

	c.mux = http.NewServeMux() // Using default mux creates problem in unit tests
	c.mux.Handle("/", crossdock.Handler(c.Behaviors, true))

	listener, err := net.Listen("tcp", c.ClientHostPort)
	if err != nil {
		return err
	}
	c.listener = listener
	c.ClientHostPort = listener.Addr().String() // override in case it was ":0"
	return nil
}

// Close stops the client
func (c *Client) Close() error {
	return c.listener.Close()
}

// URL returns a URL that the client can be accessed on
func (c *Client) URL() string {
	return fmt.Sprintf("http://%s/", c.ClientHostPort)
}

func (c *Client) setDefaultPort(port *string, defaultPort string) {
	if *port == "" {
		*port = defaultPort
	}
}
