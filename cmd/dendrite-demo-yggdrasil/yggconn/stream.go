package yggconn

import (
	"net"

	"github.com/lucas-clemente/quic-go"
)

type QUICStream struct {
	quic.Stream
	session quic.Session
}

func (s QUICStream) LocalAddr() net.Addr {
	return s.session.LocalAddr()
}

func (s QUICStream) RemoteAddr() net.Addr {
	return s.session.RemoteAddr()
}
