package conn

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/setup"
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
	_, err := pRouter.AuthenticatedConnect(parent, "static", pineconeRouter.PeerTypeRemote)
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
	tr := &http.Transport{
		DisableKeepAlives: false,
		Dial:              s.Dial,
		DialContext:       s.DialContext,
		DialTLS:           s.DialTLS,
		DialTLSContext:    s.DialTLSContext,
	}
	tr.RegisterProtocol(
		"matrix", &RoundTripper{
			inner: &http.Transport{
				DisableKeepAlives: false,
				Dial:              s.Dial,
				DialContext:       s.DialContext,
				DialTLS:           s.DialTLS,
				DialTLSContext:    s.DialTLSContext,
			},
		},
	)
	return tr
}

func CreateClient(
	base *setup.BaseDendrite, s *pineconeSessions.Sessions,
) *gomatrixserverlib.Client {
	return gomatrixserverlib.NewClient(
		gomatrixserverlib.WithTransport(createTransport(s)),
	)
}

func CreateFederationClient(
	base *setup.BaseDendrite, s *pineconeSessions.Sessions,
) *gomatrixserverlib.FederationClient {
	return gomatrixserverlib.NewFederationClient(
		base.Cfg.Global.ServerName,
		base.Cfg.Global.KeyID,
		base.Cfg.Global.PrivateKey,
		gomatrixserverlib.WithTransport(createTransport(s)),
	)
}
