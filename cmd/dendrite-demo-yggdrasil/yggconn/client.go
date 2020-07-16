package yggconn

import (
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/gomatrixserverlib"
)

type yggroundtripper struct {
	inner *http.Transport
}

func (y *yggroundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	return y.inner.RoundTrip(req)
}

func (n *Node) CreateClient(
	base *setup.BaseDendrite,
) *gomatrixserverlib.Client {
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &yggroundtripper{
			inner: &http.Transport{
				TLSHandshakeTimeout:   20 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				IdleConnTimeout:       60 * time.Second,
				DialContext:           n.DialerContext,
			},
		},
	)
	return gomatrixserverlib.NewClientWithTransport(tr)
}

func (n *Node) CreateFederationClient(
	base *setup.BaseDendrite,
) *gomatrixserverlib.FederationClient {
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &yggroundtripper{
			inner: &http.Transport{
				TLSHandshakeTimeout:   20 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				IdleConnTimeout:       60 * time.Second,
				DialContext:           n.DialerContext,
				TLSClientConfig:       n.tlsConfig,
			},
		},
	)
	return gomatrixserverlib.NewFederationClientWithTransport(
		base.Cfg.Matrix.ServerName, base.Cfg.Matrix.KeyID, base.Cfg.Matrix.PrivateKey, tr,
	)
}
