// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package conduit

import (
	"io"
	"net"
	"sync"

	"github.com/matrix-org/pinecone/types"
	"go.uber.org/atomic"
)

type Conduit struct {
	closed    atomic.Bool
	conn      net.Conn
	portMutex sync.Mutex
	port      types.SwitchPortID
}

func NewConduit(conn net.Conn, port int) Conduit {
	return Conduit{
		conn: conn,
		port: types.SwitchPortID(port),
	}
}

func (c *Conduit) Port() int {
	c.portMutex.Lock()
	defer c.portMutex.Unlock()
	return int(c.port)
}

func (c *Conduit) SetPort(port types.SwitchPortID) {
	c.portMutex.Lock()
	defer c.portMutex.Unlock()
	c.port = port
}

func (c *Conduit) Read(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, io.EOF
	}
	return c.conn.Read(b)
}

func (c *Conduit) ReadCopy() ([]byte, error) {
	if c.closed.Load() {
		return nil, io.EOF
	}
	var buf [65535 * 2]byte
	n, err := c.conn.Read(buf[:])
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (c *Conduit) Write(b []byte) (int, error) {
	if c.closed.Load() {
		return 0, io.EOF
	}
	return c.conn.Write(b)
}

func (c *Conduit) Close() error {
	if c.closed.Load() {
		return io.ErrClosedPipe
	}
	c.closed.Store(true)
	return c.conn.Close()
}
