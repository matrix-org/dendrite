// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package yggconn

import (
	"context"
	"net"
)

func (n *Node) listenFromYgg() {
	for {
		conn, err := n.utpSocket.Accept()
		if err != nil {
			n.log.Println("n.utpSocket.Accept:", err)
			return
		}
		n.incoming <- conn
	}
}

// Implements net.Listener
func (n *Node) Accept() (net.Conn, error) {
	return <-n.incoming, nil
}

// Implements net.Listener
func (n *Node) Close() error {
	return n.utpSocket.Close()
}

// Implements net.Listener
func (n *Node) Addr() net.Addr {
	return n.utpSocket.Addr()
}

// Implements http.Transport.Dial
func (n *Node) Dial(network, address string) (net.Conn, error) {
	return n.DialContext(context.TODO(), network, address)
}

// Implements http.Transport.DialContext
func (n *Node) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return n.utpSocket.DialContext(ctx, network, address)
}
