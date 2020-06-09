package yggconn

import (
	"net"
	"time"

	"github.com/alecthomas/multiplex"
)

type channel struct {
	*multiplex.Channel
	conn net.Conn
}

func (c *channel) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *channel) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *channel) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *channel) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *channel) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
