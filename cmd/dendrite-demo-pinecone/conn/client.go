// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only
// Please see LICENSE in the repository root for full details.

package conn

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib/fclient"
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
	s *pineconeSessions.Sessions,
) *fclient.Client {
	return fclient.NewClient(
		fclient.WithTransport(createTransport(s)),
	)
}

func CreateFederationClient(
	cfg *config.Dendrite, s *pineconeSessions.Sessions,
) fclient.FederationClient {
	return fclient.NewFederationClient(
		cfg.Global.SigningIdentities(),
		fclient.WithTransport(createTransport(s)),
	)
}
