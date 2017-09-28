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
	"strings"
)

// CheckEmptyExchangesConn checks whether all exchanges for the given connection are empty.
// If there are exchanges, a string with information about leftover exchanges is returned.
func CheckEmptyExchangesConn(c *ConnectionRuntimeState) string {
	var errors []string
	checkExchange := func(e ExchangeSetRuntimeState) {
		if e.Count > 0 {
			errors = append(errors, fmt.Sprintf(" %v leftover %v exchanges", e.Name, e.Count))
			for _, v := range e.Exchanges {
				errors = append(errors, fmt.Sprintf("  exchanges: %+v", v))
			}
		}
	}
	checkExchange(c.InboundExchange)
	checkExchange(c.OutboundExchange)
	if len(errors) == 0 {
		return ""
	}

	return fmt.Sprintf("Connection %d has leftover exchanges:\n\t%v", c.ID, strings.Join(errors, "\n\t"))
}

// CheckEmptyExchangesConns checks that all exchanges for the given connections are empty.
func CheckEmptyExchangesConns(connections []*ConnectionRuntimeState) string {
	var errors []string
	for _, c := range connections {
		if v := CheckEmptyExchangesConn(c); v != "" {
			errors = append(errors, v)
		}
	}
	return strings.Join(errors, "\n")
}

// CheckEmptyExchanges checks that all exchanges for the given channel are empty.
//
// TODO: Remove CheckEmptyExchanges and friends in favor of
// testutils.TestServer's verification.
func CheckEmptyExchanges(ch *Channel) string {
	state := ch.IntrospectState(&IntrospectionOptions{IncludeExchanges: true})
	var connections []*ConnectionRuntimeState
	for _, peer := range state.RootPeers {
		for _, conn := range peer.InboundConnections {
			connections = append(connections, &conn)
		}
		for _, conn := range peer.OutboundConnections {
			connections = append(connections, &conn)
		}
	}
	return CheckEmptyExchangesConns(connections)
}
