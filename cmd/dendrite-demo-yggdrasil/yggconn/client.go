package yggconn

import (
	"net/http"
	"time"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib/fclient"
)

type yggroundtripper struct {
	inner *http.Transport
}

func (y *yggroundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	return y.inner.RoundTrip(req)
}

func (n *Node) CreateClient() *fclient.Client {
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
	return fclient.NewClient(
		fclient.WithTransport(tr),
	)
}

func (n *Node) CreateFederationClient(
	cfg *config.Dendrite,
) fclient.FederationClient {
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
	return fclient.NewFederationClient(
		cfg.Global.SigningIdentities(),
		fclient.WithTransport(tr),
	)
}
