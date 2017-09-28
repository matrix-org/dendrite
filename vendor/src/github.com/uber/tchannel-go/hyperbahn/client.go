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

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/uber/tchannel-go"
	htypes "github.com/uber/tchannel-go/hyperbahn/gen-go/hyperbahn"
	tjson "github.com/uber/tchannel-go/json"
	tthrift "github.com/uber/tchannel-go/thrift"
)

// Client manages Hyperbahn connections and registrations.
type Client struct {
	tchan    *tchannel.Channel
	services []string
	opts     ClientOptions
	quit     chan struct{}

	jsonClient      *tjson.Client
	hyperbahnClient htypes.TChanHyperbahn
}

// FailStrategy is the strategy to use when registration fails maxRegistrationFailures
// times consecutively in the background. This is not used if the initial registration fails.
type FailStrategy int

const (
	// FailStrategyFatal will call Fatalf on the channel's logger after triggering handler.OnError.
	// This is the default strategy.
	FailStrategyFatal FailStrategy = iota
	// FailStrategyIgnore will only call handler.OnError, even after many
	// errors, and will continue to retry forever.
	FailStrategyIgnore
)

const hyperbahnServiceName = "hyperbahn"

// UnmarshalText implements encoding/text.Unmarshaler.
// This allows FailStrategy to be specified as a string in many
// file formats (e.g. JSON, YAML, TOML).
func (f *FailStrategy) UnmarshalText(text []byte) error {
	switch strategy := string(text); strategy {
	case "", "fatal":
		*f = FailStrategyFatal
	case "ignore":
		*f = FailStrategyIgnore
	default:
		return fmt.Errorf("not a valid fail strategy: %q", strategy)
	}
	return nil
}

// ClientOptions are used to configure this Hyperbahn client.
type ClientOptions struct {
	// Timeout defaults to 3 seconds if it is not set.
	Timeout time.Duration
	// TimeoutPerAttempt defaults to 1 second if it is not set.
	TimeoutPerAttempt time.Duration
	Handler           Handler
	FailStrategy      FailStrategy

	// The following are variables for stubbing in unit tests.
	// They are not part of the stable API and may change.
	TimeSleep func(d time.Duration)
}

// NewClient creates a new Hyperbahn client using the given channel.
// config is the environment-specific configuration for Hyperbahn such as the list of initial nodes.
// opts are optional, and are used to customize the client.
func NewClient(ch *tchannel.Channel, config Configuration, opts *ClientOptions) (*Client, error) {
	client := &Client{tchan: ch, quit: make(chan struct{})}
	if opts != nil {
		client.opts = *opts
	}
	if client.opts.Timeout == 0 {
		client.opts.Timeout = 3 * time.Second
	}
	if client.opts.TimeoutPerAttempt == 0 {
		client.opts.TimeoutPerAttempt = time.Second
	}
	if client.opts.Handler == nil {
		client.opts.Handler = nullHandler{}
	}
	if client.opts.TimeSleep == nil {
		client.opts.TimeSleep = time.Sleep
	}

	if err := parseConfig(&config); err != nil {
		return nil, err
	}

	// Add the given initial nodes as peers.
	for _, node := range config.InitialNodes {
		addPeer(ch, node)
	}

	client.jsonClient = tjson.NewClient(ch, hyperbahnServiceName, nil)
	thriftClient := tthrift.NewClient(ch, hyperbahnServiceName, nil)
	client.hyperbahnClient = htypes.NewTChanHyperbahnClient(thriftClient)

	return client, nil
}

// parseConfig parses the configuration options (e.g. InitialNodesFile)
func parseConfig(config *Configuration) error {
	if config.InitialNodesFile != "" {
		f, err := os.Open(config.InitialNodesFile)
		if err != nil {
			return err
		}
		defer f.Close()

		decoder := json.NewDecoder(f)
		if err := decoder.Decode(&config.InitialNodes); err != nil {
			return err
		}
	}

	if len(config.InitialNodes) == 0 {
		return fmt.Errorf("hyperbahn Client requires at least one initial node")
	}

	for _, node := range config.InitialNodes {
		if _, _, err := net.SplitHostPort(node); err != nil {
			return fmt.Errorf("hyperbahn Client got invalid node %v: %v", node, err)
		}
	}

	return nil
}

// addPeer adds a peer to the Hyperbahn subchannel.
// TODO(prashant): Start connections to the peers in the background.
func addPeer(ch *tchannel.Channel, hostPort string) {
	peers := ch.GetSubChannel(hyperbahnServiceName).Peers()
	peers.Add(hostPort)
}

func (c *Client) getServiceNames(otherServices []tchannel.Registrar) {
	c.services = make([]string, 0, len(otherServices)+1)
	c.services = append(c.services, c.tchan.PeerInfo().ServiceName)

	for _, s := range otherServices {
		c.services = append(c.services, s.ServiceName())
	}
}

// Advertise advertises the service with Hyperbahn, and returns any errors on initial advertisement.
// Advertise can register multiple services hosted on the same endpoint.
// If the advertisement succeeds, a goroutine is started to re-advertise periodically.
func (c *Client) Advertise(otherServices ...tchannel.Registrar) error {
	c.getServiceNames(otherServices)

	if err := c.initialAdvertise(); err != nil {
		return err
	}

	c.opts.Handler.On(Advertised)
	go c.advertiseLoop()
	return nil
}

// IsClosed returns whether this Client is closed.
func (c *Client) IsClosed() bool {
	select {
	case <-c.quit:
		return true
	default:
		return false
	}
}

// Close closes the Hyperbahn client, which stops any background re-advertisements.
func (c *Client) Close() {
	if !c.IsClosed() {
		close(c.quit)
	}
}
