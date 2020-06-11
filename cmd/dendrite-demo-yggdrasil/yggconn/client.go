package yggconn

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/convert"
	"github.com/matrix-org/dendrite/internal/basecomponent"
	"github.com/matrix-org/gomatrixserverlib"
)

type yggroundtripper struct {
	inner *http.Transport
}

func (y *yggroundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	return y.inner.RoundTrip(req)
}

func (n *Node) CreateFederationClient(
	base *basecomponent.BaseDendrite,
) *gomatrixserverlib.FederationClient {
	yggdialer := func(_, address string) (net.Conn, error) {
		tokens := strings.Split(address, ":")
		raw, err := hex.DecodeString(tokens[0])
		if err != nil {
			return nil, fmt.Errorf("hex.DecodeString: %w", err)
		}
		converted := convert.Ed25519PublicKeyToCurve25519(ed25519.PublicKey(raw))
		convhex := hex.EncodeToString(converted)
		return n.Dial("curve25519", convhex)
	}
	yggdialerctx := func(ctx context.Context, network, address string) (net.Conn, error) {
		return yggdialer(network, address)
	}
	tr := &http.Transport{}
	tr.RegisterProtocol(
		"matrix", &yggroundtripper{
			inner: &http.Transport{
				ResponseHeaderTimeout: 15 * time.Second,
				IdleConnTimeout:       60 * time.Second,
				DialContext:           yggdialerctx,
			},
		},
	)
	return gomatrixserverlib.NewFederationClientWithTransport(
		base.Cfg.Matrix.ServerName, base.Cfg.Matrix.KeyID, base.Cfg.Matrix.PrivateKey, tr,
	)
}
