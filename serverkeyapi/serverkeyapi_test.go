package serverkeyapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type server struct {
	name      gomatrixserverlib.ServerName
	validity  time.Duration
	config    *config.Dendrite
	fedclient *gomatrixserverlib.FederationClient
	cache     *caching.Caches
	api       api.ServerKeyInternalAPI
}

var (
	serverKeyID = gomatrixserverlib.KeyID("ed25519:auto")
	serverA     = &server{name: "a.com", validity: time.Duration(0)} // expires now
	serverB     = &server{name: "b.com", validity: time.Hour}        // expires in an hour
	serverC     = &server{name: "c.com", validity: -time.Hour}       // expired an hour ago
)

var servers = map[string]*server{
	"a.com": serverA,
	"b.com": serverB,
	"c.com": serverC,
}

func TestMain(m *testing.M) {
	for _, s := range servers {
		_, testPriv, err := ed25519.GenerateKey(nil)
		if err != nil {
			panic("can't generate identity key: " + err.Error())
		}

		s.cache, err = caching.NewInMemoryLRUCache(false)
		if err != nil {
			panic("can't create cache: " + err.Error())
		}

		s.config = &config.Dendrite{}
		s.config.SetDefaults()
		s.config.Matrix.KeyValidityPeriod = s.validity
		s.config.Matrix.ServerName = gomatrixserverlib.ServerName(s.name)
		s.config.Matrix.PrivateKey = testPriv
		s.config.Matrix.KeyID = serverKeyID
		s.config.Database.ServerKey = config.DataSource("file::memory:")

		transport := &http.Transport{}
		transport.RegisterProtocol("matrix", &MockRoundTripper{})

		s.fedclient = gomatrixserverlib.NewFederationClientWithTransport(
			s.config.Matrix.ServerName, serverKeyID, testPriv, transport,
		)

		s.api = NewInternalAPI(s.config, s.fedclient, s.cache)
	}

	//os.Exit(m.Run())
}

type MockRoundTripper struct{}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	s, ok := servers[req.Host]
	if !ok {
		return nil, fmt.Errorf("server not known: %s", req.Host)
	}

	request := &api.QueryLocalKeysRequest{}
	response := &api.QueryLocalKeysResponse{}
	if err = s.api.QueryLocalKeys(context.Background(), request, response); err != nil {
		return nil, err
	}

	body, err := json.MarshalIndent(response.ServerKeys, "", "  ")
	if err != nil {
		return nil, err
	}

	fmt.Println("Round-tripper says:", string(body))

	res = &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewReader(body)),
	}
	return
}

func TestServersRequestOwnKeys(t *testing.T) {
	/*
		Each server will request its own keys. There's no reason
		for this to fail as each server should know its own keys.
	*/

	for name, s := range servers {
		req := gomatrixserverlib.PublicKeyLookupRequest{
			ServerName: s.name,
			KeyID:      serverKeyID,
		}
		res, err := s.api.FetchKeys(
			context.Background(),
			map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp{
				req: gomatrixserverlib.AsTimestamp(time.Now()),
			},
		)
		if err != nil {
			t.Fatalf("server could not fetch own key: %s", err)
		}
		if _, ok := res[req]; !ok {
			t.Fatalf("server didn't return its own key in the results")
		}
		fmt.Printf("%s's key expires at %d\n", name, res[req].ValidUntilTS)
	}
}

func TestServerARequestsServerBKey(t *testing.T) {
	/*
		Server A will request Server B's key, which has a validity
		period of an hour from now. We should retrieve the key and
		it should make it into the cache automatically.
	*/

	req := gomatrixserverlib.PublicKeyLookupRequest{
		ServerName: serverB.name,
		KeyID:      serverKeyID,
	}

	res, err := serverA.api.FetchKeys(
		context.Background(),
		map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp{
			req: gomatrixserverlib.AsTimestamp(time.Now()),
		},
	)
	if err != nil {
		t.Fatalf("server A failed to retrieve server B key: %s", err)
	}
	if len(res) != 1 {
		t.Fatalf("server B should have returned one key but instead returned %d keys", len(res))
	}
	if _, ok := res[req]; !ok {
		t.Fatalf("server B isn't included in the key fetch response")
	}

	/*
		At this point, if the previous key request was a success,
		then the cache should now contain the key. Check if that's
		the case - if it isn't then there's something wrong with
		the cache implementation.
	*/

	cres, ok := serverA.cache.GetServerKey(req)
	if !ok {
		t.Fatalf("server B key should be in cache but isn't")
	}
	if !reflect.DeepEqual(cres, res[req]) {
		t.Fatalf("the cached result from server B wasn't what server B gave us")
	}

	/*
		Server A will then request Server B's key for an event that
		happened two hours ago, which *should* pass since the key was
		valid then too and it's already in the cache.
	*/

	_, err = serverA.api.FetchKeys(
		context.Background(),
		map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp{
			req: gomatrixserverlib.AsTimestamp(time.Now().Add(-time.Hour * 2)),
		},
	)
	if err != nil {
		t.Fatalf("server A failed to retrieve server B key: %s", err)
	}
}

func TestServerARequestsServerCKey(t *testing.T) {
	/*
		Server A will request Server C's key for an event that came
		in just now, but their validity period is an hour in the
		past. This *should* fail since the key isn't valid now.
	*/

	req := gomatrixserverlib.PublicKeyLookupRequest{
		ServerName: serverC.name,
		KeyID:      serverKeyID,
	}

	res, err := serverA.api.FetchKeys(
		context.Background(),
		map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp{
			req: gomatrixserverlib.AsTimestamp(time.Now()),
		},
	)
	if err != nil {
		t.Fatalf("server A failed to retrieve server C key: %s", err)
	}
	if len(res) != 1 {
		t.Fatalf("server C should have returned one key but instead returned %d keys", len(res))
	}
	if _, ok := res[req]; !ok {
		t.Fatalf("server C isn't included in the key fetch response")
	}

	t.Log("server C's key expires at", res[req].ValidUntilTS.Time())

	/*
		Server A will then request Server C's key for an event that
		happened two hours ago, which *should* pass since the key was
		valid then.
	*/

	_, err = serverA.api.FetchKeys(
		context.Background(),
		map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp{
			req: gomatrixserverlib.AsTimestamp(time.Now().Add(-time.Hour)),
		},
	)
	if err != nil {
		t.Fatalf("serverKeyAPI.FetchKeys: %s", err)
	}
}
