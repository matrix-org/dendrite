// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package conn

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/gomatrixserverlib"
	"nhooyr.io/websocket"

	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

func ConnectToPeer(pRouter *pineconeRouter.Router, peer string) error {
	var parent net.Conn
	if strings.HasPrefix(peer, "ws://") || strings.HasPrefix(peer, "wss://") {
		ctx := context.Background()
		c, _, err := websocket.Dial(ctx, peer, nil)
		if err != nil {
			return fmt.Errorf("websocket.DefaultDialer.Dial: %w", err)
		}
		parent = websocket.NetConn(ctx, c, websocket.MessageBinary)
	} else {
		var err error
		parent, err = net.Dial("tcp", peer)
		if err != nil {
			return fmt.Errorf("net.Dial: %w", err)
		}
	}
	if parent == nil {
		return fmt.Errorf("failed to wrap connection")
	}
	_, err := pRouter.Connect(
		parent,
		pineconeRouter.ConnectionZone("static"),
		pineconeRouter.ConnectionPeerType(pineconeRouter.PeerTypeRemote),
		pineconeRouter.ConnectionURI(peer),
	)
	return err
}

type RoundTripper struct {
	inner *http.Transport
}

func (y *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	return y.inner.RoundTrip(req)
}

func createTransport(s *pineconeSessions.Sessions) *http.Transport {
	proto := s.Protocol("matrix")
	tr := &http.Transport{
		DisableKeepAlives: false,
		Dial:              proto.Dial,
		DialContext:       proto.DialContext,
		DialTLS:           proto.DialTLS,
		DialTLSContext:    proto.DialTLSContext,
	}
	tr.RegisterProtocol(
		"matrix", &RoundTripper{
			inner: &http.Transport{
				DisableKeepAlives: false,
				Dial:              proto.Dial,
				DialContext:       proto.DialContext,
				DialTLS:           proto.DialTLS,
				DialTLSContext:    proto.DialTLSContext,
			},
		},
	)
	return tr
}

func CreateClient(
	base *base.BaseDendrite, s *pineconeSessions.Sessions,
) *gomatrixserverlib.Client {
	return gomatrixserverlib.NewClient(
		gomatrixserverlib.WithTransport(createTransport(s)),
	)
}

func CreateFederationClient(
	base *base.BaseDendrite, s *pineconeSessions.Sessions,
) *gomatrixserverlib.FederationClient {
	return gomatrixserverlib.NewFederationClient(
		base.Cfg.Global.ServerName,
		base.Cfg.Global.KeyID,
		base.Cfg.Global.PrivateKey,
		gomatrixserverlib.WithTransport(createTransport(s)),
	)
}
