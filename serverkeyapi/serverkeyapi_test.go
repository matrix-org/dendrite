package serverkeyapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type server struct {
	name      gomatrixserverlib.ServerName
	config    *config.Dendrite
	fedclient *gomatrixserverlib.FederationClient
	cache     *caching.Caches
	api       api.ServerKeyInternalAPI
}

var serverA = &server{name: "a.com"}
var serverB = &server{name: "b.com"}
var serverC = &server{name: "c.com"}

var servers = map[string]*server{
	"a.com": serverA,
	"b.com": serverB,
	"c.com": serverC,
}

func TestMain(m *testing.M) {
	caches, err := caching.NewInMemoryLRUCache()
	if err != nil {
		panic("can't create cache: " + err.Error())
	}

	for _, s := range []*server{serverA, serverB, serverC} {
		_, testPriv, err := ed25519.GenerateKey(nil)
		if err != nil {
			panic("can't generate identity key: " + err.Error())
		}

		s.config = &config.Dendrite{}
		s.config.SetDefaults()
		s.config.Matrix.ServerName = gomatrixserverlib.ServerName(s.name)
		s.config.Matrix.PrivateKey = testPriv
		s.config.Matrix.KeyID = "ed25519:test"
		s.config.Database.ServerKey = config.DataSource("file::memory:")

		transport := &http.Transport{}
		transport.RegisterProtocol("matrix", &MockRoundTripper{})

		s.fedclient = gomatrixserverlib.NewFederationClientWithTransport(
			s.config.Matrix.ServerName, "ed25519:test", testPriv, transport,
		)

		s.cache = caches
		s.api = NewInternalAPI(s.config, s.fedclient, s.cache)
	}
}

type MockRoundTripper struct{}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	serv, ok := servers[req.Host]
	if !ok {
		return nil, fmt.Errorf("server not known: %s", req.Host)
	}

	keys := routing.LocalKeys(serv.config).JSON
	body, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return nil, err
	}
	res = &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewReader(body)),
	}
	return
}

func TestServerKeyAPIDirect(t *testing.T) {
	res, err := serverA.api.FetchKeys(
		context.Background(),
		map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp{
			{
				ServerName: serverA.name,
				KeyID:      "ed25519:test",
			}: gomatrixserverlib.AsTimestamp(time.Now()),
		},
	)
	if err != nil {
		t.Fatalf("serverKeyAPI.FetchKeys: %s", err)
	}
	t.Log(res)
}
