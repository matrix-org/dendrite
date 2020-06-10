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
	"strings"
	"time"

	"github.com/libp2p/go-yamux"
)

func (n *Node) yamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = time.Second * 30
	cfg.MaxMessageSize = 65535
	cfg.ReadBufSize = 655350
	return cfg
}

func (n *Node) listenFromYgg() {
	for {
		conn, err := n.listener.Accept()
		if err != nil {
			n.log.Println("n.listener.Accept:", err)
			return
		}
		var session *yamux.Session
		if strings.Compare(n.EncryptionPublicKey(), conn.RemoteAddr().String()) < 0 {
			session, err = yamux.Client(conn, n.yamuxConfig())
		} else {
			session, err = yamux.Server(conn, n.yamuxConfig())
		}
		if err != nil {
			return
		}
		go n.listenFromYggConn(session)
	}
}

func (n *Node) listenFromYggConn(session *yamux.Session) {
	n.sessions.Store(session.RemoteAddr().String(), session)
	defer n.sessions.Delete(session.RemoteAddr())

	for {
		st, err := session.AcceptStream()
		if err != nil {
			n.log.Println("session.AcceptStream:", err)
			return
		}
		n.incoming <- st
	}
}

// Implements net.Listener
func (n *Node) Accept() (net.Conn, error) {
	return <-n.incoming, nil
}

// Implements net.Listener
func (n *Node) Close() error {
	return n.listener.Close()
}

// Implements net.Listener
func (n *Node) Addr() net.Addr {
	return n.listener.Addr()
}

// Implements http.Transport.Dial
func (n *Node) Dial(network, address string) (net.Conn, error) {
	return n.DialContext(context.TODO(), network, address)
}

// Implements http.Transport.DialContext
func (n *Node) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	s, ok1 := n.sessions.Load(address)
	session, ok2 := s.(*yamux.Session)
	if !ok1 || !ok2 {
		conn, err := n.dialer.DialContext(ctx, network, address)
		if err != nil {
			n.log.Println("n.dialer.DialContext:", err)
			return nil, err
		}
		if strings.Compare(n.EncryptionPublicKey(), address) < 0 {
			session, err = yamux.Client(conn, n.yamuxConfig())
		} else {
			session, err = yamux.Server(conn, n.yamuxConfig())
		}
		if err != nil {
			return nil, err
		}
		go n.listenFromYggConn(session)
	}
	st, err := session.OpenStream()
	if err != nil {
		n.log.Println("session.OpenStream:", err)
		return nil, err
	}
	return st, nil
}
