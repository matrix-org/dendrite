package yggconn

import (
	"context"
	"errors"
	"net"

	"github.com/alecthomas/multiplex"
)

func (n *Node) listenFromYgg() {
	for {
		conn, err := n.listener.Accept()
		if err != nil {
			return
		}
		stream := multiplex.MultiplexedServer(conn)
		n.conns.Store(conn.RemoteAddr(), conn)
		n.streams.Store(conn.RemoteAddr(), stream)
		go n.listenFromYggConn(stream, conn)
	}
}

func (n *Node) listenFromYggConn(stream *multiplex.MultiplexedStream, conn net.Conn) {
	for {
		ch, err := stream.Accept()
		if err != nil {
			return
		}
		n.incoming <- &channel{ch, conn}
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
		n.streams.Store(address, multiplex.MultiplexedClient(conn))
	}
	s, ok := n.streams.Load(address)
	if !ok {
		return nil, errors.New("stream not found")
	}
	stream, ok := s.(*multiplex.MultiplexedStream)
	if !ok {
		return nil, errors.New("stream type assertion error")
	}
	ch, err := stream.Dial()
	if err != nil {
		return nil, err
	}
	return &channel{ch, conn}, nil
}
