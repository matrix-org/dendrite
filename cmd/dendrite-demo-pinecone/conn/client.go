package conn

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"

	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

func ConnectToPeer(pRouter *pineconeRouter.Router, peer string) {
	var parent net.Conn
	if strings.HasPrefix(peer, "ws://") || strings.HasPrefix(peer, "wss://") {
		c, _, err := websocket.DefaultDialer.Dial(peer, nil)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to connect to Pinecone static peer %q via WebSockets", peer)
			return
		}
		parent = WrapWebSocketConn(c)
	} else {
		var err error
		parent, err = net.Dial("tcp", peer)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to connect to Pinecone static peer %q via TCP", peer)
			return
		}
	}
	if parent == nil {
		return
	}
	if _, err := pRouter.AuthenticatedConnect(parent, "static", pineconeRouter.PeerTypeRemote); err != nil {
		logrus.WithError(err).Errorf("Failed to connect Pinecone static peer to switch")
	}
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
				//MaxIdleConnsPerHost:   -1,
				DisableKeepAlives:     true,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				IdleConnTimeout:       60 * time.Second,
				Dial:                  s.Dial,
				DialContext:           s.DialContext,
				DialTLS:               s.DialTLS,
				DialTLSContext:        s.DialTLSContext,
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
				//MaxIdleConnsPerHost:   -1,
				DisableKeepAlives:     true,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				IdleConnTimeout:       60 * time.Second,
				Dial:                  s.Dial,
				DialContext:           s.DialContext,
				DialTLS:               s.DialTLS,
				DialTLSContext:        s.DialTLSContext,
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
