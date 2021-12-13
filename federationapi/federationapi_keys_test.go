package federationapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

type server struct {
	name      gomatrixserverlib.ServerName        // server name
	validity  time.Duration                       // key validity duration from now
	config    *config.FederationAPI               // skeleton config, from TestMain
	fedclient *gomatrixserverlib.FederationClient // uses MockRoundTripper
	cache     *caching.Caches                     // server-specific cache
	api       api.FederationInternalAPI           // server-specific server key API
}

func (s *server) renew() {
	// This updates the validity period to be an hour in the
	// future, which is particularly useful in server A and
	// server C's cases which have validity either as now or
	// in the past.
	s.validity = time.Hour
	s.config.Matrix.KeyValidityPeriod = s.validity
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
	// Set up the server key API for each "server" that we
	// will use in our tests.
	for _, s := range servers {
		// Generate a new key.
		_, testPriv, err := ed25519.GenerateKey(nil)
		if err != nil {
			panic("can't generate identity key: " + err.Error())
		}

		// Create a new cache but don't enable prometheus!
		s.cache, err = caching.NewInMemoryLRUCache(false)
		if err != nil {
			panic("can't create cache: " + err.Error())
		}

		// Draw up just enough Dendrite config for the server key
		// API to work.
		cfg := &config.Dendrite{}
		cfg.Defaults(true)
		cfg.Global.ServerName = gomatrixserverlib.ServerName(s.name)
		cfg.Global.PrivateKey = testPriv
		cfg.Global.Kafka.UseNaffka = true
		cfg.Global.KeyID = serverKeyID
		cfg.Global.KeyValidityPeriod = s.validity
		cfg.FederationAPI.Database.ConnectionString = config.DataSource("file::memory:")
		s.config = &cfg.FederationAPI

		// Create a transport which redirects federation requests to
		// the mock round tripper. Since we're not *really* listening for
		// federation requests then this will return the key instead.
		transport := &http.Transport{}
		transport.RegisterProtocol("matrix", &MockRoundTripper{})

		// Create the federation client.
		s.fedclient = gomatrixserverlib.NewFederationClient(
			s.config.Matrix.ServerName, serverKeyID, testPriv,
			gomatrixserverlib.WithTransport(transport),
		)

		// Finally, build the server key APIs.
		sbase := base.NewBaseDendrite(cfg, "Monolith", base.NoCacheMetrics)
		s.api = NewInternalAPI(sbase, s.fedclient, nil, s.cache, nil, true)
	}

	// Now that we have built our server key APIs, start the
	// rest of the tests.
	os.Exit(m.Run())
}

type MockRoundTripper struct{}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (res *http.Response, err error) {
	// Check if the request is looking for keys from a server that
	// we know about in the test. The only reason this should go wrong
	// is if the test is broken.
	s, ok := servers[req.Host]
	if !ok {
		return nil, fmt.Errorf("server not known: %s", req.Host)
	}

	// We're intercepting /matrix/key/v2/server requests here, so check
	// that the URL supplied in the request is for that.
	if req.URL.Path != "/_matrix/key/v2/server" {
		return nil, fmt.Errorf("unexpected request path: %s", req.URL.Path)
	}

	// Get the keys and JSON-ify them.
	keys := routing.LocalKeys(s.config)
	body, err := json.MarshalIndent(keys.JSON, "", "  ")
	if err != nil {
		return nil, err
	}

	// And respond.
	res = &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(bytes.NewReader(body)),
	}
	return
}

func TestServersRequestOwnKeys(t *testing.T) {
	// Each server will request its own keys. There's no reason
	// for this to fail as each server should know its own keys.

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
		t.Logf("%s's key expires at %s\n", name, res[req].ValidUntilTS.Time())
	}
}

func TestCachingBehaviour(t *testing.T) {
	// Server A will request Server B's key, which has a validity
	// period of an hour from now. We should retrieve the key and
	// it should make it into the cache automatically.

	req := gomatrixserverlib.PublicKeyLookupRequest{
		ServerName: serverB.name,
		KeyID:      serverKeyID,
	}
	ts := gomatrixserverlib.AsTimestamp(time.Now())

	res, err := serverA.api.FetchKeys(
		context.Background(),
		map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp{
			req: ts,
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

	// At this point, if the previous key request was a success,
	// then the cache should now contain the key. Check if that's
	// the case - if it isn't then there's something wrong with
	// the cache implementation or we failed to get the key.

	cres, ok := serverA.cache.GetServerKey(req, ts)
	if !ok {
		t.Fatalf("server B key should be in cache but isn't")
	}
	if !reflect.DeepEqual(cres, res[req]) {
		t.Fatalf("the cached result from server B wasn't what server B gave us")
	}

	// If we ask the cache for the same key but this time for an event
	// that happened in +30 minutes. Since the validity period is for
	// another hour, then we should get a response back from the cache.

	_, ok = serverA.cache.GetServerKey(
		req,
		gomatrixserverlib.AsTimestamp(time.Now().Add(time.Minute*30)),
	)
	if !ok {
		t.Fatalf("server B key isn't in cache when it should be (+30 minutes)")
	}

	// If we ask the cache for the same key but this time for an event
	// that happened in +90 minutes then we should expect to get no
	// cache result. This is because the cache shouldn't return a result
	// that is obviously past the validity of the event.

	_, ok = serverA.cache.GetServerKey(
		req,
		gomatrixserverlib.AsTimestamp(time.Now().Add(time.Minute*90)),
	)
	if ok {
		t.Fatalf("server B key is in cache when it shouldn't be (+90 minutes)")
	}
}

func TestRenewalBehaviour(t *testing.T) {
	// Server A will request Server C's key but their validity period
	// is an hour in the past. We'll retrieve the key as, even though it's
	// past its validity, it will be able to verify past events.

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

	// If we ask the cache for the server key for an event that happened
	// 90 minutes ago then we should get a cache result, as the key hadn't
	// passed its validity by that point. The fact that the key is now in
	// the cache is, in itself, proof that we successfully retrieved the
	// key before.

	oldcached, ok := serverA.cache.GetServerKey(
		req,
		gomatrixserverlib.AsTimestamp(time.Now().Add(-time.Minute*90)),
	)
	if !ok {
		t.Fatalf("server C key isn't in cache when it should be (-90 minutes)")
	}

	// If we now ask the cache for the same key but this time for an event
	// that only happened 30 minutes ago then we shouldn't get a cached
	// result, as the event happened after the key validity expired. This
	// is really just for sanity checking.

	_, ok = serverA.cache.GetServerKey(
		req,
		gomatrixserverlib.AsTimestamp(time.Now().Add(-time.Minute*30)),
	)
	if ok {
		t.Fatalf("server B key is in cache when it shouldn't be (-30 minutes)")
	}

	// We're now going to kick server C into renewing its key. Since we're
	// happy at this point that the key that we already have is from the past
	// then repeating a key fetch should cause us to try and renew the key.
	// If so, then the new key will end up in our cache.

	serverC.renew()

	res, err = serverA.api.FetchKeys(
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
	if _, ok = res[req]; !ok {
		t.Fatalf("server C isn't included in the key fetch response")
	}

	// We're now going to ask the cache what the new key validity is. If
	// it is still the same as the previous validity then we've failed to
	// retrieve the renewed key. If it's newer then we've successfully got
	// the renewed key.

	newcached, ok := serverA.cache.GetServerKey(
		req,
		gomatrixserverlib.AsTimestamp(time.Now().Add(-time.Minute*30)),
	)
	if !ok {
		t.Fatalf("server B key isn't in cache when it shouldn't be (post-renewal)")
	}
	if oldcached.ValidUntilTS >= newcached.ValidUntilTS {
		t.Fatalf("the server B key should have been renewed but wasn't")
	}
	t.Log(res)
}
