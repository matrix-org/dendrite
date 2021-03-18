package conn

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/gomatrixserverlib"

	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

func ConnectToPeer(pRouter *pineconeRouter.Router, peer string) error {
	var parent net.Conn
	if strings.HasPrefix(peer, "ws://") || strings.HasPrefix(peer, "wss://") {
		c, _, err := websocket.DefaultDialer.Dial(peer, nil)
		if err != nil {
			return fmt.Errorf("websocket.DefaultDialer.Dial: %w", err)
		}
		parent = WrapWebSocketConn(c)
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

func CreateClient(
	base *setup.BaseDendrite, s *pineconeSessions.Sessions,
) *gomatrixserverlib.Client {
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &RoundTripper{
			inner: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 5,
				Dial:                s.Dial,
				DialContext:         s.DialContext,
				DialTLS:             s.DialTLS,
				DialTLSContext:      s.DialTLSContext,
			},
		},
	)
	return gomatrixserverlib.NewClient(
		gomatrixserverlib.WithTransport(tr),
	)
}

func CreateFederationClient(
	base *setup.BaseDendrite, s *pineconeSessions.Sessions,
) *gomatrixserverlib.FederationClient {
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &RoundTripper{
			inner: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 5,
				Dial:                s.Dial,
				DialContext:         s.DialContext,
				DialTLS:             s.DialTLS,
				DialTLSContext:      s.DialTLSContext,
			},
		},
	)
	return gomatrixserverlib.NewFederationClient(
		base.Cfg.Global.ServerName,
		base.Cfg.Global.KeyID,
		base.Cfg.Global.PrivateKey,
		gomatrixserverlib.WithTransport(tr),
	)
}
