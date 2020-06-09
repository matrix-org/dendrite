package yggconn

import (
	"context"
	"errors"
	"net"

	"github.com/libp2p/go-yamux"
)

func (n *Node) listenFromYgg() {
	for {
		conn, err := n.listener.Accept()
		if err != nil {
			return
		}
		session, err := yamux.Server(conn, nil)
		if err != nil {
			return
		}
		n.conns.Store(conn.RemoteAddr(), conn)
		n.sessions.Store(conn.RemoteAddr(), session)
		go n.listenFromYggConn(session, conn)
	}
}

func (n *Node) listenFromYggConn(session *yamux.Session, conn net.Conn) {
	for {
		st, err := session.AcceptStream()
		if err != nil {
			return
		}
		n.incoming <- &stream{st, conn}
	}
}

func (n *Node) Accept() (net.Conn, error) {
	return <-n.incoming, nil
}

func (n *Node) Close() error {
	return n.listener.Close()
}

func (n *Node) Addr() net.Addr {
	return n.listener.Addr()
}

func (n *Node) Dial(network, address string) (net.Conn, error) {
	return n.DialContext(context.TODO(), network, address)
}

func (n *Node) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := n.dialer.DialContext(ctx, network, address)
	if err != nil {
		c, ok := n.conns.Load(address)
		if !ok {
			return nil, errors.New("conn not found")
		}
		conn, ok = c.(net.Conn)
		if !ok {
			return nil, errors.New("conn type assertion error")
		}
	} else {
		client, cerr := yamux.Client(conn, nil)
		if cerr != nil {
			return nil, cerr
		}
		n.sessions.Store(address, client)
	}
	s, ok := n.sessions.Load(address)
	if !ok {
		return nil, errors.New("session not found")
	}
	session, ok := s.(*yamux.Session)
	if !ok {
		return nil, errors.New("session type assertion error")
	}
	ch, err := session.OpenStream()
	if err != nil {
		return nil, err
	}
	return &stream{ch, conn}, nil
}
