package appservice_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/appservice/inthttp"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi"

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

			if r.URL.Path[len(r.URL.Path)-3:] == existingProtocol {
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
			if r.URL.Path[len(r.URL.Path)-3:] == existingProtocol {
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
			if r.URL.Path[len(r.URL.Path)-3:] == existingProtocol {
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

	// only one DBType, since appservice.AddInternalRoutes complains about multiple prometheus counters added
	base, closeBase := testrig.CreateBaseDendrite(t, test.DBTypeSQLite)
	defer closeBase()

	// Create a dummy application service
	base.Cfg.AppServiceAPI.Derived.ApplicationServices = []config.ApplicationService{
		{
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
		},
	}

	// Create required internal APIs
	rsAPI := roomserver.NewInternalAPI(base)
	usrAPI := userapi.NewInternalAPI(base, &base.Cfg.UserAPI, nil, nil, rsAPI, nil)
	asAPI := appservice.NewInternalAPI(base, usrAPI, rsAPI)

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

	// Finally execute the tests
	t.Run("HTTP API", func(t *testing.T) {
		router := mux.NewRouter().PathPrefix(httputil.InternalPathPrefix).Subrouter()
		appservice.AddInternalRoutes(router, asAPI)
		apiURL, cancel := test.ListenAndServe(t, router, false)
		defer cancel()

		asHTTPApi, err := inthttp.NewAppserviceClient(apiURL, &http.Client{})
		if err != nil {
			t.Fatalf("failed to create HTTP client: %s", err)
		}
		runCases(t, asHTTPApi)
	})

	t.Run("Monolith", func(t *testing.T) {
		runCases(t, asAPI)
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
