package yggconn

import (
	"context"
	"net"
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
		session, err := yamux.Server(conn, n.yamuxConfig())
		if err != nil {
			n.log.Println("yamux.Server:", err)
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
		session, err = yamux.Client(conn, n.yamuxConfig())
		if err != nil {
			n.log.Println("yamux.Client.AcceptStream:", err)
			return nil, err
		}
		go n.listenFromYggConn(session)
	}
	st, err := session.OpenStream()
	if err != nil {
		return nil, err
	}
	return st, nil
}
