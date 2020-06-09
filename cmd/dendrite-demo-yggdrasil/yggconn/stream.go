package yggconn

import (
	"net"
	"time"

	"github.com/libp2p/go-yamux"
)

type stream struct {
	*yamux.Stream
	conn net.Conn
}

func (c *stream) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *stream) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *stream) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *stream) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *stream) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
