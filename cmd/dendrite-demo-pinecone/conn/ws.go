// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package conn

import (
	"io"
	"net"
	"time"

	"github.com/gorilla/websocket"
)

func WrapWebSocketConn(c *websocket.Conn) *WebSocketConn {
	return &WebSocketConn{c: c}
}

type WebSocketConn struct {
	r io.Reader
	c *websocket.Conn
}

func (c *WebSocketConn) Write(p []byte) (int, error) {
	err := c.c.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *WebSocketConn) Read(p []byte) (int, error) {
	for {
		if c.r == nil {
			// Advance to next message.
			var err error
			_, c.r, err = c.c.NextReader()
			if err != nil {
				return 0, err
			}
		}
		n, err := c.r.Read(p)
		if err == io.EOF {
			// At end of message.
			c.r = nil
			if n > 0 {
				return n, nil
			} else {
				// No data read, continue to next message.
				continue
			}
		}
		return n, err
	}
}

func (c *WebSocketConn) Close() error {
	return c.c.Close()
}

func (c *WebSocketConn) LocalAddr() net.Addr {
	return c.c.LocalAddr()
}

func (c *WebSocketConn) RemoteAddr() net.Addr {
	return c.c.RemoteAddr()
}

func (c *WebSocketConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	if err := c.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (c *WebSocketConn) SetReadDeadline(t time.Time) error {
	return c.c.SetReadDeadline(t)
}

func (c *WebSocketConn) SetWriteDeadline(t time.Time) error {
	return c.c.SetWriteDeadline(t)
}
