package appservice_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/element-hq/dendrite/clientapi"
	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/federationapi/statistics"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi"
	uapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"

	"github.com/element-hq/dendrite/appservice"
	"github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/appservice/consumers"
	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver"
	rsapi "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/element-hq/dendrite/test/testrig"
)

var testIsBlacklistedOrBackingOff = func(s spec.ServerName) (*statistics.ServerStatistics, error) {
	return &statistics.ServerStatistics{}, nil
}

func TestAppserviceInternalAPI(t *testing.T) {

	// Set expected results
	existingProtocol := "irc"
	wantLocationResponse := []api.ASLocationResponse{{Protocol: existingProtocol, Fields: []byte("{}")}}
	wantUserResponse := []api.ASUserResponse{{Protocol: existingProtocol, Fields: []byte("{}")}}
	wantProtocolResponse := api.ASProtocolResponse{Instances: []api.ProtocolInstance{{Fields: []byte("{}")}}}
	wantProtocolResult := map[string]api.ASProtocolResponse{
		existingProtocol: wantProtocolResponse,
	}

	// create a dummy AS url, handling some cases
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "location"):
			// Check if we've got an existing protocol, if so, return a proper response.
			if r.URL.Path[len(r.URL.Path)-len(existingProtocol):] == existingProtocol {
				if err := json.NewEncoder(w).Encode(wantLocationResponse); err != nil {
					t.Fatalf("failed to encode response: %s", err)
				}
				return
			}
			if err := json.NewEncoder(w).Encode([]api.ASLocationResponse{}); err != nil {
				t.Fatalf("failed to encode response: %s", err)
			}
			return
		case strings.Contains(r.URL.Path, "user"):
			if r.URL.Path[len(r.URL.Path)-len(existingProtocol):] == existingProtocol {
				if err := json.NewEncoder(w).Encode(wantUserResponse); err != nil {
					t.Fatalf("failed to encode response: %s", err)
				}
				return
			}
			if err := json.NewEncoder(w).Encode([]api.UserResponse{}); err != nil {
				t.Fatalf("failed to encode response: %s", err)
			}
			return
		case strings.Contains(r.URL.Path, "protocol"):
			if r.URL.Path[len(r.URL.Path)-len(existingProtocol):] == existingProtocol {
				if err := json.NewEncoder(w).Encode(wantProtocolResponse); err != nil {
					t.Fatalf("failed to encode response: %s", err)
				}
				return
			}
			if err := json.NewEncoder(w).Encode(nil); err != nil {
				t.Fatalf("failed to encode response: %s", err)
			}
			return
		default:
			t.Logf("hit location: %s", r.URL.Path)
		}
	}))

	// The test cases to run
	runCases := func(t *testing.T, testAPI api.AppServiceInternalAPI) {
		t.Run("UserIDExists", func(t *testing.T) {
			testUserIDExists(t, testAPI, "@as-testing:test", true)
			testUserIDExists(t, testAPI, "@as1-testing:test", false)
		})

		t.Run("AliasExists", func(t *testing.T) {
			testAliasExists(t, testAPI, "@asroom-testing:test", true)
			testAliasExists(t, testAPI, "@asroom1-testing:test", false)
		})

		t.Run("Locations", func(t *testing.T) {
			testLocations(t, testAPI, existingProtocol, wantLocationResponse)
			testLocations(t, testAPI, "abc", nil)
		})

		t.Run("User", func(t *testing.T) {
			testUser(t, testAPI, existingProtocol, wantUserResponse)
			testUser(t, testAPI, "abc", nil)
		})

		t.Run("Protocols", func(t *testing.T) {
			testProtocol(t, testAPI, existingProtocol, wantProtocolResult)
			testProtocol(t, testAPI, existingProtocol, wantProtocolResult) // tests the cache
			testProtocol(t, testAPI, "", wantProtocolResult)               // tests getting all protocols
			testProtocol(t, testAPI, "abc", nil)
		})
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, ctx, close := testrig.CreateConfig(t, dbType)
		defer close()

		// Create a dummy application service
		as := &config.ApplicationService{
			ID:              "someID",
			URL:             srv.URL,
			ASToken:         "",
			HSToken:         "",
			SenderLocalpart: "senderLocalPart",
			NamespaceMap: map[string][]config.ApplicationServiceNamespace{
				"users":   {{RegexpObject: regexp.MustCompile("as-.*")}},
				"aliases": {{RegexpObject: regexp.MustCompile("asroom-.*")}},
			},
			Protocols: []string{existingProtocol},
		}
		as.CreateHTTPClient(cfg.AppServiceAPI.DisableTLSValidation)
		cfg.AppServiceAPI.Derived.ApplicationServices = []config.ApplicationService{*as}
		t.Cleanup(func() {
			ctx.ShutdownDendrite()
			ctx.WaitForShutdown()
		})
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		// Create required internal APIs
		natsInstance := jetstream.NATSInstance{}
		cm := sqlutil.NewConnectionManager(ctx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(ctx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		usrAPI := userapi.NewInternalAPI(ctx, cfg, cm, &natsInstance, rsAPI, nil, caching.DisableMetrics, testIsBlacklistedOrBackingOff)
		asAPI := appservice.NewInternalAPI(ctx, cfg, &natsInstance, usrAPI, rsAPI)

		runCases(t, asAPI)
	})
}

func TestAppserviceInternalAPI_UnixSocket_Simple(t *testing.T) {

	// Set expected results
	existingProtocol := "irc"
	wantLocationResponse := []api.ASLocationResponse{{Protocol: existingProtocol, Fields: []byte("{}")}}
	wantUserResponse := []api.ASUserResponse{{Protocol: existingProtocol, Fields: []byte("{}")}}
	wantProtocolResponse := api.ASProtocolResponse{Instances: []api.ProtocolInstance{{Fields: []byte("{}")}}}

	// create a dummy AS url, handling some cases
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "location"):
			// Check if we've got an existing protocol, if so, return a proper response.
			if r.URL.Path[len(r.URL.Path)-len(existingProtocol):] == existingProtocol {
				if err := json.NewEncoder(w).Encode(wantLocationResponse); err != nil {
					t.Fatalf("failed to encode response: %s", err)
				}
				return
			}
			if err := json.NewEncoder(w).Encode([]api.ASLocationResponse{}); err != nil {
				t.Fatalf("failed to encode response: %s", err)
			}
			return
		case strings.Contains(r.URL.Path, "user"):
			if r.URL.Path[len(r.URL.Path)-len(existingProtocol):] == existingProtocol {
				if err := json.NewEncoder(w).Encode(wantUserResponse); err != nil {
					t.Fatalf("failed to encode response: %s", err)
				}
				return
			}
			if err := json.NewEncoder(w).Encode([]api.UserResponse{}); err != nil {
				t.Fatalf("failed to encode response: %s", err)
			}
			return
		case strings.Contains(r.URL.Path, "protocol"):
			if r.URL.Path[len(r.URL.Path)-len(existingProtocol):] == existingProtocol {
				if err := json.NewEncoder(w).Encode(wantProtocolResponse); err != nil {
					t.Fatalf("failed to encode response: %s", err)
				}
				return
			}
			if err := json.NewEncoder(w).Encode(nil); err != nil {
				t.Fatalf("failed to encode response: %s", err)
			}
			return
		default:
			t.Logf("hit location: %s", r.URL.Path)
		}
	}))

	tmpDir := t.TempDir()
	socket := path.Join(tmpDir, "socket")
	l, err := net.Listen("unix", socket)
	assert.NoError(t, err)
	_ = srv.Listener.Close()
	srv.Listener = l
	srv.Start()
	defer srv.Close()

	cfg, ctx, tearDown := testrig.CreateConfig(t, test.DBTypeSQLite)
	defer tearDown()

	// Create a dummy application service
	as := &config.ApplicationService{
		ID:              "someID",
		URL:             fmt.Sprintf("unix://%s", socket),
		ASToken:         "",
		HSToken:         "",
		SenderLocalpart: "senderLocalPart",
		NamespaceMap: map[string][]config.ApplicationServiceNamespace{
			"users":   {{RegexpObject: regexp.MustCompile("as-.*")}},
			"aliases": {{RegexpObject: regexp.MustCompile("asroom-.*")}},
		},
		Protocols: []string{existingProtocol},
	}
	as.CreateHTTPClient(cfg.AppServiceAPI.DisableTLSValidation)
	cfg.AppServiceAPI.Derived.ApplicationServices = []config.ApplicationService{*as}

	t.Cleanup(func() {
		ctx.ShutdownDendrite()
		ctx.WaitForShutdown()
	})
	caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
	// Create required internal APIs
	natsInstance := jetstream.NATSInstance{}
	cm := sqlutil.NewConnectionManager(ctx, cfg.Global.DatabaseOptions)
	rsAPI := roomserver.NewInternalAPI(ctx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
	rsAPI.SetFederationAPI(nil, nil)
	usrAPI := userapi.NewInternalAPI(ctx, cfg, cm, &natsInstance, rsAPI, nil, caching.DisableMetrics, testIsBlacklistedOrBackingOff)
	asAPI := appservice.NewInternalAPI(ctx, cfg, &natsInstance, usrAPI, rsAPI)

	t.Run("UserIDExists", func(t *testing.T) {
		testUserIDExists(t, asAPI, "@as-testing:test", true)
		testUserIDExists(t, asAPI, "@as1-testing:test", false)
	})

}

func testUserIDExists(t *testing.T, asAPI api.AppServiceInternalAPI, userID string, wantExists bool) {
	ctx := context.Background()
	userResp := &api.UserIDExistsResponse{}

	if err := asAPI.UserIDExists(ctx, &api.UserIDExistsRequest{
		UserID: userID,
	}, userResp); err != nil {
		t.Errorf("failed to get userID: %s", err)
	}
	if userResp.UserIDExists != wantExists {
		t.Errorf("unexpected result for UserIDExists(%s): %v, expected %v", userID, userResp.UserIDExists, wantExists)
	}
}

func testAliasExists(t *testing.T, asAPI api.AppServiceInternalAPI, alias string, wantExists bool) {
	ctx := context.Background()
	aliasResp := &api.RoomAliasExistsResponse{}

	if err := asAPI.RoomAliasExists(ctx, &api.RoomAliasExistsRequest{
		Alias: alias,
	}, aliasResp); err != nil {
		t.Errorf("failed to get alias: %s", err)
	}
	if aliasResp.AliasExists != wantExists {
		t.Errorf("unexpected result for RoomAliasExists(%s): %v, expected %v", alias, aliasResp.AliasExists, wantExists)
	}
}

func testLocations(t *testing.T, asAPI api.AppServiceInternalAPI, proto string, wantResult []api.ASLocationResponse) {
	ctx := context.Background()
	locationResp := &api.LocationResponse{}

	if err := asAPI.Locations(ctx, &api.LocationRequest{
		Protocol: proto,
	}, locationResp); err != nil {
		t.Errorf("failed to get locations: %s", err)
	}
	if !reflect.DeepEqual(locationResp.Locations, wantResult) {
		t.Errorf("unexpected result for Locations(%s): %+v, expected %+v", proto, locationResp.Locations, wantResult)
	}
}

func testUser(t *testing.T, asAPI api.AppServiceInternalAPI, proto string, wantResult []api.ASUserResponse) {
	ctx := context.Background()
	userResp := &api.UserResponse{}

	if err := asAPI.User(ctx, &api.UserRequest{
		Protocol: proto,
	}, userResp); err != nil {
		t.Errorf("failed to get user: %s", err)
	}
	if !reflect.DeepEqual(userResp.Users, wantResult) {
		t.Errorf("unexpected result for User(%s): %+v, expected %+v", proto, userResp.Users, wantResult)
	}
}

func testProtocol(t *testing.T, asAPI api.AppServiceInternalAPI, proto string, wantResult map[string]api.ASProtocolResponse) {
	ctx := context.Background()
	protoResp := &api.ProtocolResponse{}

	if err := asAPI.Protocols(ctx, &api.ProtocolRequest{
		Protocol: proto,
	}, protoResp); err != nil {
		t.Errorf("failed to get Protocols: %s", err)
	}
	if !reflect.DeepEqual(protoResp.Protocols, wantResult) {
		t.Errorf("unexpected result for Protocols(%s): %+v, expected %+v", proto, protoResp.Protocols[proto], wantResult)
	}
}

// Tests that the roomserver consumer only receives one invite
func TestRoomserverConsumerOneInvite(t *testing.T) {

	alice := test.NewUser(t)
	bob := test.NewUser(t)
	room := test.NewRoom(t, alice)

	// Invite Bob
	room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
		"membership": "invite",
	}, test.WithStateKey(bob.ID))

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, closeDB := testrig.CreateConfig(t, dbType)
		defer closeDB()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		natsInstance := &jetstream.NATSInstance{}

		evChan := make(chan struct{})
		// create a dummy AS url, handling the events
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var txn consumers.ApplicationServiceTransaction
			err := json.NewDecoder(r.Body).Decode(&txn)
			if err != nil {
				t.Fatal(err)
			}
			for _, ev := range txn.Events {
				if ev.Type != spec.MRoomMember {
					continue
				}
				// Usually we would check the event content for the membership, but since
				// we only invited bob, this should be fine for this test.
				if ev.StateKey != nil && *ev.StateKey == bob.ID {
					evChan <- struct{}{}
				}
			}
		}))
		defer srv.Close()

		as := &config.ApplicationService{
			ID:              "someID",
			URL:             srv.URL,
			ASToken:         "",
			HSToken:         "",
			SenderLocalpart: "senderLocalPart",
			NamespaceMap: map[string][]config.ApplicationServiceNamespace{
				"users":   {{RegexpObject: regexp.MustCompile(bob.ID)}},
				"aliases": {{RegexpObject: regexp.MustCompile(room.ID)}},
			},
		}
		as.CreateHTTPClient(cfg.AppServiceAPI.DisableTLSValidation)

		// Create a dummy application service
		cfg.AppServiceAPI.Derived.ApplicationServices = []config.ApplicationService{*as}

		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		// Create required internal APIs
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		usrAPI := userapi.NewInternalAPI(processCtx, cfg, cm, natsInstance, rsAPI, nil, caching.DisableMetrics, testIsBlacklistedOrBackingOff)
		// start the consumer
		appservice.NewInternalAPI(processCtx, cfg, natsInstance, usrAPI, rsAPI)

		// Create the room
		if err := rsapi.SendEvents(context.Background(), rsAPI, rsapi.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatalf("failed to send events: %v", err)
		}
		var seenInvitesForBob int
	waitLoop:
		for {
			select {
			case <-time.After(time.Millisecond * 50): // wait for the AS to process the events
				break waitLoop
			case <-evChan:
				seenInvitesForBob++
				if seenInvitesForBob != 1 {
					t.Fatalf("received unexpected invites: %d", seenInvitesForBob)
				}
			}
		}
		close(evChan)
	})
}

// Note: If this test panics, it is because we timed out waiting for the
// join event to come through to the appservice and we close the DB/shutdown Dendrite. This makes the
// syncAPI unhappy, as it is unable to write to the database.
func TestOutputAppserviceEvent(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, closeDB := testrig.CreateConfig(t, dbType)
		defer closeDB()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		natsInstance := &jetstream.NATSInstance{}

		evChan := make(chan struct{})

		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		// Create required internal APIs
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)

		// Create the router, so we can hit `/joined_members`
		routers := httputil.NewRouters()

		accessTokens := map[*test.User]userDevice{
			bob: {},
		}

		usrAPI := userapi.NewInternalAPI(processCtx, cfg, cm, natsInstance, rsAPI, nil, caching.DisableMetrics, testIsBlacklistedOrBackingOff)
		clientapi.AddPublicRoutes(processCtx, routers, cfg, natsInstance, nil, rsAPI, nil, nil, nil, usrAPI, nil, nil, caching.DisableMetrics)
		createAccessTokens(t, accessTokens, usrAPI, processCtx.Context(), routers)

		room := test.NewRoom(t, alice)

		// Invite Bob
		room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
			"membership": "invite",
		}, test.WithStateKey(bob.ID))

		// create a dummy AS url, handling the events
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var txn consumers.ApplicationServiceTransaction
			err := json.NewDecoder(r.Body).Decode(&txn)
			if err != nil {
				t.Fatal(err)
			}
			for _, ev := range txn.Events {
				if ev.Type != spec.MRoomMember {
					continue
				}
				if ev.StateKey != nil && *ev.StateKey == bob.ID {
					membership := gjson.GetBytes(ev.Content, "membership").Str
					t.Logf("Processing membership: %s", membership)
					switch membership {
					case spec.Invite:
						// Accept the invite
						joinEv := room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
							"membership": "join",
						}, test.WithStateKey(bob.ID))

						if err := rsapi.SendEvents(context.Background(), rsAPI, rsapi.KindNew, []*types.HeaderedEvent{joinEv}, "test", "test", "test", nil, false); err != nil {
							t.Fatalf("failed to send events: %v", err)
						}
					case spec.Join: // the AS has received the join event, now hit `/joined_members` to validate that
						rec := httptest.NewRecorder()
						req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/rooms/"+room.ID+"/joined_members", nil)
						req.Header.Set("Authorization", "Bearer "+accessTokens[bob].accessToken)
						routers.Client.ServeHTTP(rec, req)
						if rec.Code != http.StatusOK {
							t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
						}

						// Both Alice and Bob should be joined. If not, we have a race condition
						if !gjson.GetBytes(rec.Body.Bytes(), "joined."+alice.ID).Exists() {
							t.Errorf("Alice is not joined to the room") // in theory should not happen
						}
						if !gjson.GetBytes(rec.Body.Bytes(), "joined."+bob.ID).Exists() {
							t.Errorf("Bob is not joined to the room")
						}
						evChan <- struct{}{}
					default:
						t.Fatalf("Unexpected membership: %s", membership)
					}
				}
			}
		}))
		defer srv.Close()

		as := &config.ApplicationService{
			ID:              "someID",
			URL:             srv.URL,
			ASToken:         "",
			HSToken:         "",
			SenderLocalpart: "senderLocalPart",
			NamespaceMap: map[string][]config.ApplicationServiceNamespace{
				"users":   {{RegexpObject: regexp.MustCompile(bob.ID)}},
				"aliases": {{RegexpObject: regexp.MustCompile(room.ID)}},
			},
		}
		as.CreateHTTPClient(cfg.AppServiceAPI.DisableTLSValidation)

		// Create a dummy application service
		cfg.AppServiceAPI.Derived.ApplicationServices = []config.ApplicationService{*as}

		// Prepare AS Streams on the old topic to validate that they get deleted
		jsCtx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)

		token := jetstream.Tokenise(as.ID)
		if err := jetstream.JetStreamConsumer(
			processCtx.Context(), jsCtx, cfg.Global.JetStream.Prefixed(jetstream.OutputRoomEvent),
			cfg.Global.JetStream.Durable("Appservice_"+token),
			50, // maximum number of events to send in a single transaction
			func(ctx context.Context, msgs []*nats.Msg) bool {
				return true
			},
		); err != nil {
			t.Fatal(err)
		}

		// Start the syncAPI to have `/joined_members` available
		syncapi.AddPublicRoutes(processCtx, routers, cfg, cm, natsInstance, usrAPI, rsAPI, caches, caching.DisableMetrics)

		// start the consumer
		appservice.NewInternalAPI(processCtx, cfg, natsInstance, usrAPI, rsAPI)

		// At this point, the old JetStream consumers should be deleted
		for consumer := range jsCtx.Consumers(cfg.Global.JetStream.Prefixed(jetstream.OutputRoomEvent)) {
			if consumer.Name == cfg.Global.JetStream.Durable("Appservice_"+token)+"Pull" {
				t.Fatalf("Consumer still exists")
			}
		}

		// Create the room, this triggers the AS to receive an invite for Bob.
		if err := rsapi.SendEvents(context.Background(), rsAPI, rsapi.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatalf("failed to send events: %v", err)
		}

		select {
		// Pretty generous timeout duration...
		case <-time.After(time.Millisecond * 1000): // wait for the AS to process the events
			t.Errorf("Timed out waiting for join event")
		case <-evChan:
		}
		close(evChan)
	})
}

type userDevice struct {
	accessToken string
	deviceID    string
	password    string
}

func createAccessTokens(t *testing.T, accessTokens map[*test.User]userDevice, userAPI uapi.UserInternalAPI, ctx context.Context, routers httputil.Routers) {
	t.Helper()
	for u := range accessTokens {
		localpart, serverName, _ := gomatrixserverlib.SplitID('@', u.ID)
		userRes := &uapi.PerformAccountCreationResponse{}
		password := util.RandomString(8)
		if err := userAPI.PerformAccountCreation(ctx, &uapi.PerformAccountCreationRequest{
			AccountType: u.AccountType,
			Localpart:   localpart,
			ServerName:  serverName,
			Password:    password,
		}, userRes); err != nil {
			t.Errorf("failed to create account: %s", err)
		}
		req := test.NewRequest(t, http.MethodPost, "/_matrix/client/v3/login", test.WithJSONBody(t, map[string]interface{}{
			"type": authtypes.LoginTypePassword,
			"identifier": map[string]interface{}{
				"type": "m.id.user",
				"user": u.ID,
			},
			"password": password,
		}))
		rec := httptest.NewRecorder()
		routers.Client.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("failed to login: %s", rec.Body.String())
		}
		accessTokens[u] = userDevice{
			accessToken: gjson.GetBytes(rec.Body.Bytes(), "access_token").String(),
			deviceID:    gjson.GetBytes(rec.Body.Bytes(), "device_id").String(),
			password:    password,
		}
	}
}
