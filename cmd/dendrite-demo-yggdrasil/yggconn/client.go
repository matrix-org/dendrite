package yggconn

import (
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/setup"
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
				MaxIdleConns:          -1,
				MaxIdleConnsPerHost:   -1,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				IdleConnTimeout:       30 * time.Second,
				DialContext:           n.DialerContext,
			},
		},
	)
	return gomatrixserverlib.NewClient(
		gomatrixserverlib.WithTransport(tr),
	)
}

func (n *Node) CreateFederationClient(
	base *setup.BaseDendrite,
) *gomatrixserverlib.FederationClient {
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &yggroundtripper{
			inner: &http.Transport{
				MaxIdleConns:          -1,
				MaxIdleConnsPerHost:   -1,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				IdleConnTimeout:       30 * time.Second,
				DialContext:           n.DialerContext,
				TLSClientConfig:       n.tlsConfig,
			},
		},
	)
	return gomatrixserverlib.NewFederationClient(
		base.Cfg.Global.ServerName, base.Cfg.Global.KeyID,
		base.Cfg.Global.PrivateKey,
		gomatrixserverlib.WithTransport(tr),
	)
}
