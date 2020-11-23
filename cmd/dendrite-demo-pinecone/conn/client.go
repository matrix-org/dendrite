package conn

import (
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/gomatrixserverlib"

	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

type RoundTripper struct {
	inner *http.Transport
}

func (y *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	return y.inner.RoundTrip(req)
}

func CreateClient(
	base *setup.BaseDendrite, quic *pineconeSessions.QUIC,
) *gomatrixserverlib.Client {
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &RoundTripper{
			inner: &http.Transport{
				MaxIdleConnsPerHost:   1,
				DisableKeepAlives:     true,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				IdleConnTimeout:       5 * time.Second,
				Dial:                  quic.Dial,
				DialContext:           quic.DialContext,
				DialTLS:               quic.DialTLS,
				DialTLSContext:        quic.DialTLSContext,
			},
		},
	)
	return gomatrixserverlib.NewClientWithTransport(tr)
}

func CreateFederationClient(
	base *setup.BaseDendrite, quic *pineconeSessions.QUIC,
) *gomatrixserverlib.FederationClient {
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &RoundTripper{
			inner: &http.Transport{
				MaxIdleConnsPerHost:   1,
				DisableKeepAlives:     true,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				IdleConnTimeout:       5 * time.Second,
				Dial:                  quic.Dial,
				DialContext:           quic.DialContext,
				DialTLS:               quic.DialTLS,
				DialTLSContext:        quic.DialTLSContext,
			},
		},
	)
	return gomatrixserverlib.NewFederationClientWithTransport(
		base.Cfg.Global.ServerName, base.Cfg.Global.KeyID,
		base.Cfg.Global.PrivateKey, true, tr,
	)
}
