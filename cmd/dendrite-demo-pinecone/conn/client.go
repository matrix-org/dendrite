package conn

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/gomatrixserverlib"

	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

func ConnectToPeer(pRouter *pineconeRouter.Router, peer string) error {
	var parent net.Conn
	dialer := net.Dialer{
		Timeout:   time.Second * 5,
		KeepAlive: time.Second * 5,
	}
	if strings.HasPrefix(peer, "ws://") || strings.HasPrefix(peer, "wss://") {
		wsdialer := websocket.Dialer{
			NetDial:          dialer.Dial,
			NetDialContext:   dialer.DialContext,
			HandshakeTimeout: time.Second * 5,
		}
		c, _, err := wsdialer.Dial(peer, nil)
		if err != nil {
			return fmt.Errorf("websocket.DefaultDialer.Dial: %w", err)
		}
		parent = WrapWebSocketConn(c)
	} else {
		var err error
		parent, err = dialer.Dial("tcp", peer)
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
	tr := &http.Transport{}
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
