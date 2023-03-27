package clientapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/tidwall/gjson"
)

func TestSetDisplayname(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	notLocalUser := &test.User{ID: "@charlie:localhost", Localpart: "charlie"}
	changeDisplayName := "my new display name"

	testCases := []struct {
		name            string
		user            *test.User
		wantOK          bool
		changeReq       io.Reader
		wantDisplayName string
	}{
		{
			name: "invalid user",
			user: &test.User{ID: "!notauser"},
		},
		{
			name: "non-existent user",
			user: &test.User{ID: "@doesnotexist:test"},
		},
		{
			name: "non-local user is not allowed",
			user: notLocalUser,
		},
		{
			name:            "existing user is allowed to change own name",
			user:            alice,
			wantOK:          true,
			wantDisplayName: changeDisplayName,
		},
		{
			name:            "existing user is now allowed to change own if name is empty",
			user:            bob,
			wantOK:          false,
			wantDisplayName: "",
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, closeDB := testrig.CreateConfig(t, dbType)
		defer closeDB()
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		natsInstance := &jetstream.NATSInstance{}

		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, natsInstance, rsAPI, nil)
		asPI := appservice.NewInternalAPI(processCtx, cfg, natsInstance, userAPI, rsAPI)

		AddPublicRoutes(processCtx, routers, cfg, natsInstance, base.CreateFederationClient(cfg, nil), rsAPI, asPI, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		accessTokens := map[*test.User]string{
			alice: "",
			bob:   "",
		}

		createAccessTokens(t, accessTokens, userAPI, processCtx.Context(), routers)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				wantDisplayName := tc.user.Localpart
				if tc.changeReq == nil {
					tc.changeReq = strings.NewReader("")
				}

				// check profile after initial account creation
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/profile/"+tc.user.ID, strings.NewReader(""))
				t.Logf("%s", req.URL.String())
				routers.Client.ServeHTTP(rec, req)

				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d", rec.Code)
				}

				if gotDisplayName := gjson.GetBytes(rec.Body.Bytes(), "displayname").Str; tc.wantOK && gotDisplayName != wantDisplayName {
					t.Fatalf("expected displayname to be '%s', but got '%s'", wantDisplayName, gotDisplayName)
				}

				// now set the new display name
				wantDisplayName = tc.wantDisplayName
				tc.changeReq = strings.NewReader(fmt.Sprintf(`{"displayname":"%s"}`, tc.wantDisplayName))

				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/profile/"+tc.user.ID+"/displayname", tc.changeReq)
				req.Header.Set("Authorization", "Bearer "+accessTokens[tc.user])

				routers.Client.ServeHTTP(rec, req)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}

				// now only get the display name
				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/profile/"+tc.user.ID+"/displayname", strings.NewReader(""))

				routers.Client.ServeHTTP(rec, req)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}

				if gotDisplayName := gjson.GetBytes(rec.Body.Bytes(), "displayname").Str; tc.wantOK && gotDisplayName != wantDisplayName {
					t.Fatalf("expected displayname to be '%s', but got '%s'", wantDisplayName, gotDisplayName)
				}
			})
		}
	})
}

func TestSetAvatarURL(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	notLocalUser := &test.User{ID: "@charlie:localhost", Localpart: "charlie"}
	changeDisplayName := "mxc://newMXID"

	testCases := []struct {
		name       string
		user       *test.User
		wantOK     bool
		changeReq  io.Reader
		avatar_url string
	}{
		{
			name: "invalid user",
			user: &test.User{ID: "!notauser"},
		},
		{
			name: "non-existent user",
			user: &test.User{ID: "@doesnotexist:test"},
		},
		{
			name: "non-local user is not allowed",
			user: notLocalUser,
		},
		{
			name:       "existing user is allowed to change own name",
			user:       alice,
			wantOK:     true,
			avatar_url: changeDisplayName,
		},
		{
			name:       "existing user is now allowed to change own if name is empty",
			user:       bob,
			wantOK:     false,
			avatar_url: "",
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, closeDB := testrig.CreateConfig(t, dbType)
		defer closeDB()
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		natsInstance := &jetstream.NATSInstance{}

		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, natsInstance, rsAPI, nil)
		asPI := appservice.NewInternalAPI(processCtx, cfg, natsInstance, userAPI, rsAPI)

		AddPublicRoutes(processCtx, routers, cfg, natsInstance, base.CreateFederationClient(cfg, nil), rsAPI, asPI, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		accessTokens := map[*test.User]string{
			alice: "",
			bob:   "{}",
		}

		createAccessTokens(t, accessTokens, userAPI, processCtx.Context(), routers)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				wantAvatarURL := ""
				if tc.changeReq == nil {
					tc.changeReq = strings.NewReader("")
				}

				// check profile after initial account creation
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/profile/"+tc.user.ID, strings.NewReader(""))
				t.Logf("%s", req.URL.String())
				routers.Client.ServeHTTP(rec, req)

				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d", rec.Code)
				}

				if gotDisplayName := gjson.GetBytes(rec.Body.Bytes(), "avatar_url").Str; tc.wantOK && gotDisplayName != wantAvatarURL {
					t.Fatalf("expected displayname to be '%s', but got '%s'", wantAvatarURL, gotDisplayName)
				}

				// now set the new display name
				wantAvatarURL = tc.avatar_url
				tc.changeReq = strings.NewReader(fmt.Sprintf(`{"avatar_url":"%s"}`, tc.avatar_url))

				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/profile/"+tc.user.ID+"/avatar_url", tc.changeReq)
				req.Header.Set("Authorization", "Bearer "+accessTokens[tc.user])

				routers.Client.ServeHTTP(rec, req)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}

				// now only get the display name
				rec = httptest.NewRecorder()
				req = httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/profile/"+tc.user.ID+"/avatar_url", strings.NewReader(""))

				routers.Client.ServeHTTP(rec, req)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}

				if gotDisplayName := gjson.GetBytes(rec.Body.Bytes(), "avatar_url").Str; tc.wantOK && gotDisplayName != wantAvatarURL {
					t.Fatalf("expected displayname to be '%s', but got '%s'", wantAvatarURL, gotDisplayName)
				}
			})
		}
	})
}
