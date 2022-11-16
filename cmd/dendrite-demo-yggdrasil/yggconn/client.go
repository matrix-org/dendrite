package yggconn

import (
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/setup/base"
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
	base *base.BaseDendrite,
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
	base *base.BaseDendrite,
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
			},
		},
	)
	return gomatrixserverlib.NewFederationClient(
		base.Cfg.Global.SigningIdentities(),
		gomatrixserverlib.WithTransport(tr),
	)
}
