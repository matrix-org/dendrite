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

	"github.com/stretchr/testify/assert"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/appservice/consumers"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/matrix-org/dendrite/test/testrig"
)

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
		usrAPI := userapi.NewInternalAPI(ctx, cfg, cm, &natsInstance, rsAPI, nil)
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
	usrAPI := userapi.NewInternalAPI(ctx, cfg, cm, &natsInstance, rsAPI, nil)
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
		usrAPI := userapi.NewInternalAPI(processCtx, cfg, cm, natsInstance, rsAPI, nil)
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
