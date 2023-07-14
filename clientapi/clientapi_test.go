package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type userDevice struct {
	accessToken string
	deviceID    string
	password    string
}

func TestGetPutDevices(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		testCases := []struct {
			name           string
			requestUser    *test.User
			deviceUser     *test.User
			request        *http.Request
			wantStatusCode int
			validateFunc   func(t *testing.T, device userDevice, routers httputil.Routers)
		}{
			{
				name:           "can get all devices",
				requestUser:    alice,
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/devices", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
			},
			{
				name:           "can get specific own device",
				requestUser:    alice,
				deviceUser:     alice,
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/devices/", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
			},
			{
				name:           "can not get device for different user",
				requestUser:    alice,
				deviceUser:     bob,
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/devices/", strings.NewReader("")),
				wantStatusCode: http.StatusNotFound,
			},
			{
				name:           "can update own device",
				requestUser:    alice,
				deviceUser:     alice,
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/devices/", strings.NewReader(`{"display_name":"my new displayname"}`)),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, device userDevice, routers httputil.Routers) {
					req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/devices/"+device.deviceID, strings.NewReader(""))
					req.Header.Set("Authorization", "Bearer "+device.accessToken)
					rec := httptest.NewRecorder()
					routers.Client.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
					}
					gotDisplayName := gjson.GetBytes(rec.Body.Bytes(), "display_name").Str
					if gotDisplayName != "my new displayname" {
						t.Fatalf("expected displayname '%s', got '%s'", "my new displayname", gotDisplayName)
					}
				},
			},
			{
				// this should return "device does not exist"
				name:           "can not update device for different user",
				requestUser:    alice,
				deviceUser:     bob,
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/devices/", strings.NewReader(`{"display_name":"my new displayname"}`)),
				wantStatusCode: http.StatusNotFound,
			},
		}

		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		natsInstance := jetstream.NATSInstance{}
		defer close()

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)

		// We mostly need the rsAPI for this test, so nil for other APIs/caches etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		accessTokens := map[*test.User]userDevice{
			alice: {},
			bob:   {},
		}
		createAccessTokens(t, accessTokens, userAPI, processCtx.Context(), routers)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				dev := accessTokens[tc.requestUser]
				if tc.deviceUser != nil {
					tc.request = httptest.NewRequest(tc.request.Method, tc.request.RequestURI+accessTokens[tc.deviceUser].deviceID, tc.request.Body)
				}
				tc.request.Header.Set("Authorization", "Bearer "+dev.accessToken)
				rec := httptest.NewRecorder()
				routers.Client.ServeHTTP(rec, tc.request)
				if rec.Code != tc.wantStatusCode {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}
				if tc.wantStatusCode != http.StatusOK && rec.Code != http.StatusOK {
					return
				}
				if tc.validateFunc != nil {
					tc.validateFunc(t, dev, routers)
				}
			})
		}
	})
}

// Deleting devices requires the UIA dance, so do this in a different test
func TestDeleteDevice(t *testing.T) {
	alice := test.NewUser(t)
	localpart, serverName, _ := gomatrixserverlib.SplitID('@', alice.ID)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, closeDB := testrig.CreateConfig(t, dbType)
		defer closeDB()

		natsInstance := jetstream.NATSInstance{}
		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)

		// We mostly need the rsAPI/ for this test, so nil for other APIs/caches etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		accessTokens := map[*test.User]userDevice{
			alice: {},
		}

		// create the account and an initial device
		createAccessTokens(t, accessTokens, userAPI, processCtx.Context(), routers)

		// create some more devices
		accessToken := util.RandomString(8)
		devRes := &uapi.PerformDeviceCreationResponse{}
		if err := userAPI.PerformDeviceCreation(processCtx.Context(), &uapi.PerformDeviceCreationRequest{
			Localpart:          localpart,
			ServerName:         serverName,
			AccessToken:        accessToken,
			NoDeviceListUpdate: true,
		}, devRes); err != nil {
			t.Fatal(err)
		}
		if !devRes.DeviceCreated {
			t.Fatalf("failed to create device")
		}
		secondDeviceID := devRes.Device.ID

		// initiate UIA for the second device
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/devices/"+secondDeviceID, strings.NewReader(""))
		req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
		routers.Client.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected HTTP 401, got %d: %s", rec.Code, rec.Body.String())
		}
		// get the session ID
		sessionID := gjson.GetBytes(rec.Body.Bytes(), "session").Str

		// prepare UIA request body
		reqBody := bytes.Buffer{}
		if err := json.NewEncoder(&reqBody).Encode(map[string]interface{}{
			"auth": map[string]string{
				"session":  sessionID,
				"type":     authtypes.LoginTypePassword,
				"user":     alice.ID,
				"password": accessTokens[alice].password,
			},
		}); err != nil {
			t.Fatal(err)
		}

		// copy the request body, so we can use it again for the successful delete
		reqBody2 := reqBody

		// do the same request again, this time with our UIA, but for a different device ID, this should fail
		rec = httptest.NewRecorder()

		req = httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/devices/"+accessTokens[alice].deviceID, &reqBody)
		req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
		routers.Client.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected HTTP 403, got %d: %s", rec.Code, rec.Body.String())
		}

		// do the same request again, this time with our UIA, but for the correct device ID, this should be fine
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/devices/"+secondDeviceID, &reqBody2)
		req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
		routers.Client.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// verify devices are deleted
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/devices", strings.NewReader(""))
		req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
		routers.Client.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
		}
		for _, device := range gjson.GetBytes(rec.Body.Bytes(), "devices.#.device_id").Array() {
			if device.Str == secondDeviceID {
				t.Fatalf("expected device %s to be deleted, but wasn't", secondDeviceID)
			}
		}
	})
}

// Deleting devices requires the UIA dance, so do this in a different test
func TestDeleteDevices(t *testing.T) {
	alice := test.NewUser(t)
	localpart, serverName, _ := gomatrixserverlib.SplitID('@', alice.ID)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, closeDB := testrig.CreateConfig(t, dbType)
		defer closeDB()

		natsInstance := jetstream.NATSInstance{}
		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)

		// We mostly need the rsAPI/ for this test, so nil for other APIs/caches etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		accessTokens := map[*test.User]userDevice{
			alice: {},
		}

		// create the account and an initial device
		createAccessTokens(t, accessTokens, userAPI, processCtx.Context(), routers)

		// create some more devices
		var devices []string
		for i := 0; i < 10; i++ {
			accessToken := util.RandomString(8)
			devRes := &uapi.PerformDeviceCreationResponse{}
			if err := userAPI.PerformDeviceCreation(processCtx.Context(), &uapi.PerformDeviceCreationRequest{
				Localpart:          localpart,
				ServerName:         serverName,
				AccessToken:        accessToken,
				NoDeviceListUpdate: true,
			}, devRes); err != nil {
				t.Fatal(err)
			}
			if !devRes.DeviceCreated {
				t.Fatalf("failed to create device")
			}
			devices = append(devices, devRes.Device.ID)
		}

		// initiate UIA
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/delete_devices", strings.NewReader(""))
		req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
		routers.Client.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected HTTP 401, got %d: %s", rec.Code, rec.Body.String())
		}
		// get the session ID
		sessionID := gjson.GetBytes(rec.Body.Bytes(), "session").Str

		// prepare UIA request body
		reqBody := bytes.Buffer{}
		if err := json.NewEncoder(&reqBody).Encode(map[string]interface{}{
			"auth": map[string]string{
				"session":  sessionID,
				"type":     authtypes.LoginTypePassword,
				"user":     alice.ID,
				"password": accessTokens[alice].password,
			},
			"devices": devices[5:],
		}); err != nil {
			t.Fatal(err)
		}

		// do the same request again, this time with our UIA,
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/delete_devices", &reqBody)
		req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
		routers.Client.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// verify devices are deleted
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/devices", strings.NewReader(""))
		req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
		routers.Client.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
		}
		for _, device := range gjson.GetBytes(rec.Body.Bytes(), "devices.#.device_id").Array() {
			for _, deletedDevice := range devices[5:] {
				if device.Str == deletedDevice {
					t.Fatalf("expected device %s to be deleted, but wasn't", deletedDevice)
				}
			}
		}
	})
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
			name:            "existing user is not allowed to change own name if name is empty",
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

		accessTokens := map[*test.User]userDevice{
			alice: {},
			bob:   {},
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
				req.Header.Set("Authorization", "Bearer "+accessTokens[tc.user].accessToken)

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
			name:       "existing user is allowed to change own avatar",
			user:       alice,
			wantOK:     true,
			avatar_url: changeDisplayName,
		},
		{
			name:       "existing user is not allowed to change own avatar if avatar is empty",
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

		accessTokens := map[*test.User]userDevice{
			alice: {},
			bob:   {},
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
				req.Header.Set("Authorization", "Bearer "+accessTokens[tc.user].accessToken)

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

func TestTyping(t *testing.T) {
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()
		natsInstance := jetstream.NATSInstance{}

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		// Needed to create accounts
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
		// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]userDevice{
			alice: {},
		}
		createAccessTokens(t, accessTokens, userAPI, ctx, routers)

		// Create the room
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatal(err)
		}

		testCases := []struct {
			name          string
			typingForUser string
			roomID        string
			requestBody   io.Reader
			wantOK        bool
		}{
			{
				name:          "can not set typing for different user",
				typingForUser: "@notourself:test",
				roomID:        room.ID,
				requestBody:   strings.NewReader(""),
			},
			{
				name:          "invalid request body",
				typingForUser: alice.ID,
				roomID:        room.ID,
				requestBody:   strings.NewReader(""),
			},
			{
				name:          "non-existent room",
				typingForUser: alice.ID,
				roomID:        "!doesnotexist:test",
			},
			{
				name:          "invalid room ID",
				typingForUser: alice.ID,
				roomID:        "@notaroomid:test",
			},
			{
				name:          "allowed to set own typing status",
				typingForUser: alice.ID,
				roomID:        room.ID,
				requestBody:   strings.NewReader(`{"typing":true}`),
				wantOK:        true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/rooms/"+tc.roomID+"/typing/"+tc.typingForUser, tc.requestBody)
				req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
				routers.Client.ServeHTTP(rec, req)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}
			})
		}
	})
}

func TestMembership(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	room := test.NewRoom(t, alice)
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cfg.ClientAPI.RateLimiting.Enabled = false
		defer close()
		natsInstance := jetstream.NATSInstance{}

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		// Needed to create accounts
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
		rsAPI.SetUserAPI(userAPI)
		// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]userDevice{
			alice: {},
			bob:   {},
		}
		createAccessTokens(t, accessTokens, userAPI, ctx, routers)

		// Create the room
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatal(err)
		}

		invalidBodyRequest := func(roomID, membershipType string) *http.Request {
			return httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", roomID, membershipType), strings.NewReader(""))
		}

		missingUserIDRequest := func(roomID, membershipType string) *http.Request {
			return httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", roomID, membershipType), strings.NewReader("{}"))
		}

		testCases := []struct {
			name    string
			roomID  string
			request *http.Request
			wantOK  bool
			asUser  *test.User
		}{
			{
				name:    "ban - invalid request body",
				request: invalidBodyRequest(room.ID, "ban"),
			},
			{
				name:    "kick - invalid request body",
				request: invalidBodyRequest(room.ID, "kick"),
			},
			{
				name:    "unban - invalid request body",
				request: invalidBodyRequest(room.ID, "unban"),
			},
			{
				name:    "invite - invalid request body",
				request: invalidBodyRequest(room.ID, "invite"),
			},
			{
				name:    "ban - missing user_id body",
				request: missingUserIDRequest(room.ID, "ban"),
			},
			{
				name:    "kick - missing user_id body",
				request: missingUserIDRequest(room.ID, "kick"),
			},
			{
				name:    "unban - missing user_id body",
				request: missingUserIDRequest(room.ID, "unban"),
			},
			{
				name:    "invite - missing user_id body",
				request: missingUserIDRequest(room.ID, "invite"),
			},
			{
				name:    "Bob forgets invalid room",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", "!doesnotexist", "forget"), strings.NewReader("")),
				asUser:  bob,
			},
			{
				name:    "Alice can not ban Bob in non-existent room", // fails because "not joined"
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", "!doesnotexist:test", "ban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
			},
			{
				name:    "Alice can not kick Bob in non-existent room", // fails because "not joined"
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", "!doesnotexist:test", "kick"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
			},
			// the following must run in sequence, as they build up on each other
			{
				name:    "Alice invites Bob",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "invite"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Bob accepts invite",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "join"), strings.NewReader("")),
				wantOK:  true,
				asUser:  bob,
			},
			{
				name:    "Alice verifies that Bob is joined", // returns an error if no membership event can be found
				request: httptest.NewRequest(http.MethodGet, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s/m.room.member/%s", room.ID, "state", bob.ID), strings.NewReader("")),
				wantOK:  true,
			},
			{
				name:    "Bob forgets the room but is still a member",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "forget"), strings.NewReader("")),
				wantOK:  false, // user is still in the room
				asUser:  bob,
			},
			{
				name:    "Bob can not kick Alice",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "kick"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, alice.ID))),
				wantOK:  false, // powerlevel too low
				asUser:  bob,
			},
			{
				name:    "Bob can not ban Alice",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "ban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, alice.ID))),
				wantOK:  false, // powerlevel too low
				asUser:  bob,
			},
			{
				name:    "Alice can kick Bob",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "kick"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Alice can ban Bob",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "ban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Alice can not kick Bob again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "kick"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  false, // can not kick banned/left user
			},
			{
				name:    "Bob can not unban himself", // mostly because of not being a member of the room
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "unban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				asUser:  bob,
			},
			{
				name:    "Alice can not invite Bob again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "invite"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  false, // user still banned
			},
			{
				name:    "Alice can unban Bob",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "unban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Alice can not unban Bob again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "unban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  false,
			},
			{
				name:    "Alice can invite Bob again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "invite"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Bob can reject the invite by leaving",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "leave"), strings.NewReader("")),
				wantOK:  true,
				asUser:  bob,
			},
			{
				name:    "Bob can forget the room",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "forget"), strings.NewReader("")),
				wantOK:  true,
				asUser:  bob,
			},
			{
				name:    "Bob can forget the room again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "forget"), strings.NewReader("")),
				wantOK:  true,
				asUser:  bob,
			},
			// END must run in sequence
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.asUser == nil {
					tc.asUser = alice
				}
				rec := httptest.NewRecorder()
				tc.request.Header.Set("Authorization", "Bearer "+accessTokens[tc.asUser].accessToken)
				routers.Client.ServeHTTP(rec, tc.request)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}
				if !tc.wantOK && rec.Code == http.StatusOK {
					t.Fatalf("expected request to fail, but didn't: %s", rec.Body.String())
				}
				t.Logf("%s", rec.Body.String())
			})
		}
	})
}

func TestCapabilities(t *testing.T) {
	alice := test.NewUser(t)
	ctx := context.Background()

	// construct the expected result
	versionsMap := map[gomatrixserverlib.RoomVersion]string{}
	for v, desc := range version.SupportedRoomVersions() {
		if desc.Stable() {
			versionsMap[v] = "stable"
		} else {
			versionsMap[v] = "unstable"
		}
	}

	expectedMap := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"m.change_password": map[string]bool{
				"enabled": true,
			},
			"m.room_versions": map[string]interface{}{
				"default":   version.DefaultRoomVersion(),
				"available": versionsMap,
			},
		},
	}

	expectedBuf := &bytes.Buffer{}
	err := json.NewEncoder(expectedBuf).Encode(expectedMap)
	assert.NoError(t, err)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cfg.ClientAPI.RateLimiting.Enabled = false
		defer close()
		natsInstance := jetstream.NATSInstance{}

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)

		// Needed to create accounts
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, nil, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
		// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]userDevice{
			alice: {},
		}
		createAccessTokens(t, accessTokens, userAPI, ctx, routers)

		testCases := []struct {
			name    string
			request *http.Request
		}{
			{
				name:    "can get capabilities",
				request: httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/capabilities", strings.NewReader("")),
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				tc.request.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
				routers.Client.ServeHTTP(rec, tc.request)
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.ObjectsAreEqual(expectedBuf.Bytes(), rec.Body.Bytes())
			})
		}
	})
}

func TestTurnserver(t *testing.T) {
	alice := test.NewUser(t)
	ctx := context.Background()

	cfg, processCtx, close := testrig.CreateConfig(t, test.DBTypeSQLite)
	cfg.ClientAPI.RateLimiting.Enabled = false
	defer close()
	natsInstance := jetstream.NATSInstance{}

	routers := httputil.NewRouters()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)

	// Needed to create accounts
	rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, nil, caching.DisableMetrics)
	rsAPI.SetFederationAPI(nil, nil)
	userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
	//rsAPI.SetUserAPI(userAPI)
	// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
	AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

	// Create the users in the userapi and login
	accessTokens := map[*test.User]userDevice{
		alice: {},
	}
	createAccessTokens(t, accessTokens, userAPI, ctx, routers)

	testCases := []struct {
		name              string
		turnConfig        config.TURN
		wantEmptyResponse bool
	}{
		{
			name:              "no turn server configured",
			wantEmptyResponse: true,
		},
		{
			name:              "servers configured but not userLifeTime",
			wantEmptyResponse: true,
			turnConfig:        config.TURN{URIs: []string{""}},
		},
		{
			name:              "missing sharedSecret/username/password",
			wantEmptyResponse: true,
			turnConfig:        config.TURN{URIs: []string{""}, UserLifetime: "1m"},
		},
		{
			name:       "with shared secret",
			turnConfig: config.TURN{URIs: []string{""}, UserLifetime: "1m", SharedSecret: "iAmSecret"},
		},
		{
			name:       "with username/password secret",
			turnConfig: config.TURN{URIs: []string{""}, UserLifetime: "1m", Username: "username", Password: "iAmSecret"},
		},
		{
			name:              "only username set",
			turnConfig:        config.TURN{URIs: []string{""}, UserLifetime: "1m", Username: "username"},
			wantEmptyResponse: true,
		},
		{
			name:              "only password set",
			turnConfig:        config.TURN{URIs: []string{""}, UserLifetime: "1m", Username: "username"},
			wantEmptyResponse: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/voip/turnServer", strings.NewReader(""))
			req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
			cfg.ClientAPI.TURN = tc.turnConfig
			routers.Client.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)

			if tc.wantEmptyResponse && rec.Body.String() != "{}" {
				t.Fatalf("expected an empty response, but got %s", rec.Body.String())
			}
			if !tc.wantEmptyResponse {
				assert.NotEqual(t, "{}", rec.Body.String())

				resp := gomatrix.RespTurnServer{}
				err := json.NewDecoder(rec.Body).Decode(&resp)
				assert.NoError(t, err)

				duration, _ := time.ParseDuration(tc.turnConfig.UserLifetime)
				assert.Equal(t, tc.turnConfig.URIs, resp.URIs)
				assert.Equal(t, int(duration.Seconds()), resp.TTL)
				if tc.turnConfig.Username != "" && tc.turnConfig.Password != "" {
					assert.Equal(t, tc.turnConfig.Username, resp.Username)
					assert.Equal(t, tc.turnConfig.Password, resp.Password)
				}
			}
		})
	}
}

func Test3PID(t *testing.T) {
	alice := test.NewUser(t)
	ctx := context.Background()

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cfg.ClientAPI.RateLimiting.Enabled = false
		cfg.FederationAPI.DisableTLSValidation = true // needed to be able to connect to our identityServer below
		defer close()
		natsInstance := jetstream.NATSInstance{}

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)

		// Needed to create accounts
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, nil, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
		// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]userDevice{
			alice: {},
		}
		createAccessTokens(t, accessTokens, userAPI, ctx, routers)

		identityServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.String(), "getValidated3pid"):
				resp := threepid.GetValidatedResponse{}
				switch r.URL.Query().Get("client_secret") {
				case "fail":
					resp.ErrCode = string(spec.ErrorSessionNotValidated)
				case "fail2":
					resp.ErrCode = "some other error"
				case "fail3":
					_, _ = w.Write([]byte("{invalidJson"))
					return
				case "success":
					resp.Medium = "email"
				case "success2":
					resp.Medium = "email"
					resp.Address = "somerandom@address.com"
				}
				_ = json.NewEncoder(w).Encode(resp)
			case strings.Contains(r.URL.String(), "requestToken"):
				resp := threepid.SID{SID: "randomSID"}
				_ = json.NewEncoder(w).Encode(resp)
			}
		}))
		defer identityServer.Close()

		identityServerBase := strings.TrimPrefix(identityServer.URL, "https://")

		testCases := []struct {
			name             string
			request          *http.Request
			wantOK           bool
			setTrustedServer bool
			wantLen3PIDs     int
		}{
			{
				name:    "can get associated threepid info",
				request: httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/account/3pid", strings.NewReader("")),
				wantOK:  true,
			},
			{
				name:    "can not set threepid info with invalid JSON",
				request: httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid", strings.NewReader("")),
			},
			{
				name:    "can not set threepid info with untrusted server",
				request: httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid", strings.NewReader("{}")),
			},
			{
				name:             "can check threepid info with trusted server, but unverified",
				request:          httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid", strings.NewReader(fmt.Sprintf(`{"three_pid_creds":{"id_server":"%s","client_secret":"fail"}}`, identityServerBase))),
				setTrustedServer: true,
				wantOK:           false,
			},
			{
				name:             "can check threepid info with trusted server, but fails for some other reason",
				request:          httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid", strings.NewReader(fmt.Sprintf(`{"three_pid_creds":{"id_server":"%s","client_secret":"fail2"}}`, identityServerBase))),
				setTrustedServer: true,
				wantOK:           false,
			},
			{
				name:             "can check threepid info with trusted server, but fails because of invalid json",
				request:          httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid", strings.NewReader(fmt.Sprintf(`{"three_pid_creds":{"id_server":"%s","client_secret":"fail3"}}`, identityServerBase))),
				setTrustedServer: true,
				wantOK:           false,
			},
			{
				name:             "can save threepid info with trusted server",
				request:          httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid", strings.NewReader(fmt.Sprintf(`{"three_pid_creds":{"id_server":"%s","client_secret":"success"}}`, identityServerBase))),
				setTrustedServer: true,
				wantOK:           true,
			},
			{
				name:             "can save threepid info with trusted server using bind=true",
				request:          httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid", strings.NewReader(fmt.Sprintf(`{"three_pid_creds":{"id_server":"%s","client_secret":"success2"},"bind":true}`, identityServerBase))),
				setTrustedServer: true,
				wantOK:           true,
			},
			{
				name:         "can get associated threepid info again",
				request:      httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/account/3pid", strings.NewReader("")),
				wantOK:       true,
				wantLen3PIDs: 2,
			},
			{
				name:    "can delete associated threepid info",
				request: httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid/delete", strings.NewReader(`{"medium":"email","address":"somerandom@address.com"}`)),
				wantOK:  true,
			},
			{
				name:         "can get associated threepid after deleting association",
				request:      httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/account/3pid", strings.NewReader("")),
				wantOK:       true,
				wantLen3PIDs: 1,
			},
			{
				name:    "can not request emailToken with invalid request body",
				request: httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid/email/requestToken", strings.NewReader("")),
			},
			{
				name:    "can not request emailToken for in use address",
				request: httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid/email/requestToken", strings.NewReader(fmt.Sprintf(`{"client_secret":"somesecret","email":"","send_attempt":1,"id_server":"%s"}`, identityServerBase))),
			},
			{
				name:    "can request emailToken",
				request: httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/account/3pid/email/requestToken", strings.NewReader(fmt.Sprintf(`{"client_secret":"somesecret","email":"somerandom@address.com","send_attempt":1,"id_server":"%s"}`, identityServerBase))),
				wantOK:  true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {

				if tc.setTrustedServer {
					cfg.Global.TrustedIDServers = []string{identityServerBase}
				}

				rec := httptest.NewRecorder()
				tc.request.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)

				routers.Client.ServeHTTP(rec, tc.request)
				t.Logf("Response: %s", rec.Body.String())
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}
				if !tc.wantOK && rec.Code == http.StatusOK {
					t.Fatalf("expected request to fail, but didn't: %s", rec.Body.String())
				}
				if tc.wantLen3PIDs > 0 {
					var resp routing.ThreePIDsResponse
					if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
						t.Fatal(err)
					}
					if len(resp.ThreePIDs) != tc.wantLen3PIDs {
						t.Fatalf("expected %d threepids, got %d", tc.wantLen3PIDs, len(resp.ThreePIDs))
					}
				}
			})
		}
	})
}

func TestPushRules(t *testing.T) {
	alice := test.NewUser(t)

	// create the default push rules, used when validating responses
	localpart, serverName, _ := gomatrixserverlib.SplitID('@', alice.ID)
	pushRuleSets := pushrules.DefaultAccountRuleSets(localpart, serverName)
	defaultRules, err := json.Marshal(pushRuleSets)
	assert.NoError(t, err)

	ruleID1 := "myrule"
	ruleID2 := "myrule2"
	ruleID3 := "myrule3"

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cfg.ClientAPI.RateLimiting.Enabled = false
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		natsInstance := jetstream.NATSInstance{}
		defer close()

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)

		// We mostly need the rsAPI for this test, so nil for other APIs/caches etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		accessTokens := map[*test.User]userDevice{
			alice: {},
		}
		createAccessTokens(t, accessTokens, userAPI, processCtx.Context(), routers)

		testCases := []struct {
			name           string
			request        *http.Request
			wantStatusCode int
			validateFunc   func(t *testing.T, respBody *bytes.Buffer) // used when updating rules, otherwise wantStatusCode should be enough
			queryAttr      map[string]string
		}{
			{
				name:           "can not get rules without trailing slash",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can get default rules",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					assert.Equal(t, defaultRules, respBody.Bytes())
				},
			},
			{
				name:           "can get rules by scope",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					assert.Equal(t, gjson.GetBytes(defaultRules, "global").Raw, respBody.String())
				},
			},
			{
				name:           "can not get invalid rules by scope",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/doesnotexist/", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not get rules for invalid scope and kind",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/doesnotexist/invalid/", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not get rules for invalid kind",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/invalid/", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can get rules by scope and kind",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/override/", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					assert.Equal(t, gjson.GetBytes(defaultRules, "global.override").Raw, respBody.String())
				},
			},
			{
				name:           "can get rules by scope and content kind",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/content/", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					assert.Equal(t, gjson.GetBytes(defaultRules, "global.content").Raw, respBody.String())
				},
			},
			{
				name:           "can not get rules by scope and room kind",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/room/", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not get rules by scope and sender kind",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/sender/", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can get rules by scope and underride kind",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/underride/", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					assert.Equal(t, gjson.GetBytes(defaultRules, "global.underride").Raw, respBody.String())
				},
			},
			{
				name:           "can not get rules by scope, kind and ID for invalid scope",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/doesnotexist/doesnotexist/.m.rule.master", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not get rules by scope, kind and ID for invalid kind",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/doesnotexist/.m.rule.master", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can get rules by scope, kind and ID",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/override/.m.rule.master", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
			},
			{
				name:           "can not get rules by scope, kind and ID for invalid ID",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/override/.m.rule.doesnotexist", strings.NewReader("")),
				wantStatusCode: http.StatusNotFound,
			},
			{
				name:           "can not get status for invalid attribute",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/override/.m.rule.master/invalid", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not get status for invalid kind",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/invalid/.m.rule.master/enabled", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not get enabled status for invalid scope",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/invalid/override/.m.rule.master/enabled", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not get enabled status for invalid rule",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/override/doesnotexist/enabled", strings.NewReader("")),
				wantStatusCode: http.StatusNotFound,
			},
			{
				name:           "can get enabled rules by scope, kind and ID",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/override/.m.rule.master/enabled", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					assert.False(t, gjson.GetBytes(respBody.Bytes(), "enabled").Bool(), "expected master rule to be disabled")
				},
			},
			{
				name:           "can get actions scope, kind and ID",
				request:        httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/override/.m.rule.master/actions", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					actions := gjson.GetBytes(respBody.Bytes(), "actions").Array()
					// only a basic check
					assert.Equal(t, 1, len(actions))
				},
			},
			{
				name:           "can not set enabled status with invalid JSON",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/override/.m.rule.master/enabled", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not set attribute for invalid attribute",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/override/.m.rule.master/doesnotexist", strings.NewReader("{}")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not set attribute for invalid scope",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/invalid/override/.m.rule.master/enabled", strings.NewReader("{}")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not set attribute for invalid kind",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/invalid/.m.rule.master/enabled", strings.NewReader("{}")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not set attribute for invalid rule",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/override/invalid/enabled", strings.NewReader("{}")),
				wantStatusCode: http.StatusNotFound,
			},
			{
				name:           "can set enabled status with valid JSON",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/override/.m.rule.master/enabled", strings.NewReader(`{"enabled":true}`)),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/override/.m.rule.master/enabled", strings.NewReader(""))
					req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
					routers.Client.ServeHTTP(rec, req)
					assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					assert.True(t, gjson.GetBytes(rec.Body.Bytes(), "enabled").Bool(), "expected master rule to be enabled: %s", rec.Body.String())
				},
			},
			{
				name:           "can set actions with valid JSON",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/override/.m.rule.master/actions", strings.NewReader(`{"actions":["dont_notify","notify"]}`)),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/override/.m.rule.master/actions", strings.NewReader(""))
					req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
					routers.Client.ServeHTTP(rec, req)
					assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					assert.Equal(t, 2, len(gjson.GetBytes(rec.Body.Bytes(), "actions").Array()), "expected 2 actions %s", rec.Body.String())
				},
			},
			{
				name:           "can not create new push rule with invalid JSON",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/content/myrule", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not create new push rule with invalid rule content",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/content/myrule", strings.NewReader("{}")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not create new push rule with invalid scope",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/invalid/content/myrule", strings.NewReader(`{"actions":["notify"],"pattern":"world"}`)),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can create new push rule with valid rule content",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/content/myrule", strings.NewReader(`{"actions":["notify"],"pattern":"world"}`)),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/content/myrule/actions", strings.NewReader(""))
					req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
					routers.Client.ServeHTTP(rec, req)
					assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					assert.Equal(t, 1, len(gjson.GetBytes(rec.Body.Bytes(), "actions").Array()), "expected 1 action %s", rec.Body.String())
				},
			},
			{
				name:           "can not create new push starting with a dot",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/content/.myrule", strings.NewReader(`{"actions":["notify"],"pattern":"world"}`)),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:    "can create new push rule after existing",
				request: httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/content/myrule2", strings.NewReader(`{"actions":["notify"],"pattern":"world"}`)),
				queryAttr: map[string]string{
					"after": ruleID1,
				},
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/content/", strings.NewReader(""))
					req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
					routers.Client.ServeHTTP(rec, req)
					assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					rules := gjson.ParseBytes(rec.Body.Bytes())
					for i, rule := range rules.Array() {
						if rule.Get("rule_id").Str == ruleID1 && i != 0 {
							t.Fatalf("expected '%s' to be the first, but wasn't", ruleID1)
						}
						if rule.Get("rule_id").Str == ruleID2 && i != 1 {
							t.Fatalf("expected '%s' to be the second, but wasn't", ruleID2)
						}
					}
				},
			},
			{
				name:    "can create new push rule before existing",
				request: httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/content/myrule3", strings.NewReader(`{"actions":["notify"],"pattern":"world"}`)),
				queryAttr: map[string]string{
					"before": ruleID1,
				},
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/content/", strings.NewReader(""))
					req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
					routers.Client.ServeHTTP(rec, req)
					assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					rules := gjson.ParseBytes(rec.Body.Bytes())
					for i, rule := range rules.Array() {
						if rule.Get("rule_id").Str == ruleID3 && i != 0 {
							t.Fatalf("expected '%s' to be the first, but wasn't", ruleID3)
						}
						if rule.Get("rule_id").Str == ruleID1 && i != 1 {
							t.Fatalf("expected '%s' to be the second, but wasn't", ruleID1)
						}
						if rule.Get("rule_id").Str == ruleID2 && i != 2 {
							t.Fatalf("expected '%s' to be the third, but wasn't", ruleID1)
						}
					}
				},
			},
			{
				name:           "can modify existing push rule",
				request:        httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/content/myrule2", strings.NewReader(`{"actions":["dont_notify"],"pattern":"world"}`)),
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/content/myrule2/actions", strings.NewReader(""))
					req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
					routers.Client.ServeHTTP(rec, req)
					assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					actions := gjson.GetBytes(rec.Body.Bytes(), "actions").Array()
					// there should only be one action
					assert.Equal(t, "dont_notify", actions[0].Str)
				},
			},
			{
				name:    "can move existing push rule to the front",
				request: httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/pushrules/global/content/myrule2", strings.NewReader(`{"actions":["dont_notify"],"pattern":"world"}`)),
				queryAttr: map[string]string{
					"before": ruleID3,
				},
				wantStatusCode: http.StatusOK,
				validateFunc: func(t *testing.T, respBody *bytes.Buffer) {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/pushrules/global/content/", strings.NewReader(""))
					req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
					routers.Client.ServeHTTP(rec, req)
					assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
					rules := gjson.ParseBytes(rec.Body.Bytes())
					for i, rule := range rules.Array() {
						if rule.Get("rule_id").Str == ruleID2 && i != 0 {
							t.Fatalf("expected '%s' to be the first, but wasn't", ruleID2)
						}
						if rule.Get("rule_id").Str == ruleID3 && i != 1 {
							t.Fatalf("expected '%s' to be the second, but wasn't", ruleID3)
						}
						if rule.Get("rule_id").Str == ruleID1 && i != 2 {
							t.Fatalf("expected '%s' to be the third, but wasn't", ruleID1)
						}
					}
				},
			},
			{
				name:           "can not delete push rule with invalid scope",
				request:        httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/pushrules/invalid/content/myrule2", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not delete push rule with invalid kind",
				request:        httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/pushrules/global/invalid/myrule2", strings.NewReader("")),
				wantStatusCode: http.StatusBadRequest,
			},
			{
				name:           "can not delete push rule with non-existent rule",
				request:        httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/pushrules/global/content/doesnotexist", strings.NewReader("")),
				wantStatusCode: http.StatusNotFound,
			},
			{
				name:           "can delete existing push rule",
				request:        httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/pushrules/global/content/myrule2", strings.NewReader("")),
				wantStatusCode: http.StatusOK,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()

				if tc.queryAttr != nil {
					params := url.Values{}
					for k, v := range tc.queryAttr {
						params.Set(k, v)
					}

					tc.request = httptest.NewRequest(tc.request.Method, tc.request.URL.String()+"?"+params.Encode(), tc.request.Body)
				}

				tc.request.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)

				routers.Client.ServeHTTP(rec, tc.request)
				assert.Equal(t, tc.wantStatusCode, rec.Code, rec.Body.String())
				if tc.validateFunc != nil {
					tc.validateFunc(t, rec.Body)
				}
				t.Logf("%s", rec.Body.String())
			})
		}
	})
}

// Tests the `/keys` endpoints.
// Note that this only tests the happy path.
func TestKeys(t *testing.T) {
	alice := test.NewUser(t)

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cfg.ClientAPI.RateLimiting.Enabled = false
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		natsInstance := jetstream.NATSInstance{}
		defer close()

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)

		// We mostly need the rsAPI for this test, so nil for other APIs/caches etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		accessTokens := map[*test.User]userDevice{
			alice: {},
		}
		createAccessTokens(t, accessTokens, userAPI, processCtx.Context(), routers)

		// Start a TLSServer with our client mux
		srv := httptest.NewTLSServer(routers.Client)
		defer srv.Close()

		cl, err := mautrix.NewClient(srv.URL, id.UserID(alice.ID), accessTokens[alice].accessToken)
		if err != nil {
			t.Fatal(err)
		}
		// Set the client so the self-signed certificate is trusted
		cl.Client = srv.Client()
		cl.DeviceID = id.DeviceID(accessTokens[alice].deviceID)

		cs := crypto.NewMemoryStore(nil)
		oc := crypto.NewOlmMachine(cl, nil, cs, dummyStore{})
		if err = oc.Load(); err != nil {
			t.Fatal(err)
		}

		// tests `/keys/upload`
		if err = oc.ShareKeys(ctx, 0); err != nil {
			t.Fatal(err)
		}

		// tests `/keys/device_signing/upload`
		_, err = oc.GenerateAndUploadCrossSigningKeys(accessTokens[alice].password, "passphrase")
		if err != nil {
			t.Fatal(err)
		}

		// tests `/keys/query`
		dev, err := oc.GetOrFetchDevice(ctx, id.UserID(alice.ID), id.DeviceID(accessTokens[alice].deviceID))
		if err != nil {
			t.Fatal(err)
		}

		// Validate that the keys returned from the server are what the client has stored
		oi := oc.OwnIdentity()
		if oi.SigningKey != dev.SigningKey {
			t.Fatalf("expected signing key '%s', got '%s'", oi.SigningKey, dev.SigningKey)
		}
		if oi.IdentityKey != dev.IdentityKey {
			t.Fatalf("expected identity '%s', got '%s'", oi.IdentityKey, dev.IdentityKey)
		}

		// tests `/keys/signatures/upload`
		if err = oc.SignOwnMasterKey(); err != nil {
			t.Fatal(err)
		}

		// tests `/keys/claim`
		otks := make(map[string]map[string]string)
		otks[alice.ID] = map[string]string{
			accessTokens[alice].deviceID: string(id.KeyAlgorithmSignedCurve25519),
		}

		data, err := json.Marshal(claimKeysRequest{OneTimeKeys: otks})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/_matrix/client/v3/keys/claim", bytes.NewBuffer(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !gjson.GetBytes(respBody, "one_time_keys."+alice.ID+"."+string(dev.DeviceID)).Exists() {
			t.Fatalf("expected one time keys for alice, but didn't find any: %s", string(respBody))
		}
	})
}

type claimKeysRequest struct {
	//  The keys to be claimed. A map from user ID, to a map from device ID to algorithm name.
	OneTimeKeys map[string]map[string]string `json:"one_time_keys"`
}

type dummyStore struct{}

func (d dummyStore) IsEncrypted(roomID id.RoomID) bool {
	return true
}

func (d dummyStore) GetEncryptionEvent(roomID id.RoomID) *event.EncryptionEventContent {
	return &event.EncryptionEventContent{}
}

func (d dummyStore) FindSharedRooms(userID id.UserID) []id.RoomID {
	return []id.RoomID{}
}

func TestKeyBackup(t *testing.T) {
	alice := test.NewUser(t)

	handleResponseCode := func(t *testing.T, rec *httptest.ResponseRecorder, expectedCode int) {
		t.Helper()
		if rec.Code != expectedCode {
			t.Fatalf("expected HTTP %d, but got %d: %s", expectedCode, rec.Code, rec.Body.String())
		}
	}

	testCases := []struct {
		name     string
		request  func(t *testing.T) *http.Request
		validate func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name: "can not create backup with invalid JSON",
			request: func(t *testing.T) *http.Request {
				reqBody := strings.NewReader(`{"algorithm":"m.megolm_backup.v1"`) // missing closing braces
				return httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/room_keys/version", reqBody)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusBadRequest)
			},
		},
		{
			name: "can not create backup with missing auth_data", // as this would result in MarshalJSON errors when querying again
			request: func(t *testing.T) *http.Request {
				reqBody := strings.NewReader(`{"algorithm":"m.megolm_backup.v1"}`)
				return httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/room_keys/version", reqBody)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusBadRequest)
			},
		},
		{
			name: "can create backup",
			request: func(t *testing.T) *http.Request {
				reqBody := strings.NewReader(`{"algorithm":"m.megolm_backup.v1","auth_data":{"data":"random"}}`)
				return httptest.NewRequest(http.MethodPost, "/_matrix/client/v3/room_keys/version", reqBody)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
				wantVersion := "1"
				if gotVersion := gjson.GetBytes(rec.Body.Bytes(), "version").Str; gotVersion != wantVersion {
					t.Fatalf("expected version '%s', got '%s'", wantVersion, gotVersion)
				}
			},
		},
		{
			name: "can not query backup for invalid version",
			request: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/room_keys/version/1337", nil)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusNotFound)
			},
		},
		{
			name: "can not query backup for invalid version string",
			request: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/room_keys/version/notanumber", nil)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusNotFound)
			},
		},
		{
			name: "can query backup",
			request: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/room_keys/version", nil)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
				wantVersion := "1"
				if gotVersion := gjson.GetBytes(rec.Body.Bytes(), "version").Str; gotVersion != wantVersion {
					t.Fatalf("expected version '%s', got '%s'", wantVersion, gotVersion)
				}
			},
		},
		{
			name: "can query backup without returning rooms",
			request: func(t *testing.T) *http.Request {
				req := test.NewRequest(t, http.MethodGet, "/_matrix/client/v3/room_keys/keys", test.WithQueryParams(map[string]string{
					"version": "1",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
				if gotRooms := gjson.GetBytes(rec.Body.Bytes(), "rooms").Map(); len(gotRooms) > 0 {
					t.Fatalf("expected no rooms in version, but got %#v", gotRooms)
				}
			},
		},
		{
			name: "can query backup for invalid room",
			request: func(t *testing.T) *http.Request {
				req := test.NewRequest(t, http.MethodGet, "/_matrix/client/v3/room_keys/keys/!abc:test", test.WithQueryParams(map[string]string{
					"version": "1",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
				if gotSessions := gjson.GetBytes(rec.Body.Bytes(), "sessions").Map(); len(gotSessions) > 0 {
					t.Fatalf("expected no sessions in version, but got %#v", gotSessions)
				}
			},
		},
		{
			name: "can not query backup for invalid session",
			request: func(t *testing.T) *http.Request {
				req := test.NewRequest(t, http.MethodGet, "/_matrix/client/v3/room_keys/keys/!abc:test/doesnotexist", test.WithQueryParams(map[string]string{
					"version": "1",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusNotFound)
			},
		},
		{
			name: "can not update backup with missing version",
			request: func(t *testing.T) *http.Request {
				return test.NewRequest(t, http.MethodPut, "/_matrix/client/v3/room_keys/keys")
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusBadRequest)
			},
		},
		{
			name: "can not update backup with invalid data",
			request: func(t *testing.T) *http.Request {
				reqBody := test.WithJSONBody(t, "")
				req := test.NewRequest(t, http.MethodPut, "/_matrix/client/v3/room_keys/keys", reqBody, test.WithQueryParams(map[string]string{
					"version": "0",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusBadRequest)
			},
		},
		{
			name: "can not update backup with wrong version",
			request: func(t *testing.T) *http.Request {
				reqBody := test.WithJSONBody(t, map[string]interface{}{
					"rooms": map[string]interface{}{
						"!testroom:test": map[string]interface{}{
							"sessions": map[string]uapi.KeyBackupSession{},
						},
					},
				})
				req := test.NewRequest(t, http.MethodPut, "/_matrix/client/v3/room_keys/keys", reqBody, test.WithQueryParams(map[string]string{
					"version": "5",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusForbidden)
			},
		},
		{
			name: "can update backup with correct version",
			request: func(t *testing.T) *http.Request {
				reqBody := test.WithJSONBody(t, map[string]interface{}{
					"rooms": map[string]interface{}{
						"!testroom:test": map[string]interface{}{
							"sessions": map[string]uapi.KeyBackupSession{
								"dummySession": {
									FirstMessageIndex: 1,
								},
							},
						},
					},
				})
				req := test.NewRequest(t, http.MethodPut, "/_matrix/client/v3/room_keys/keys", reqBody, test.WithQueryParams(map[string]string{
					"version": "1",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
			},
		},
		{
			name: "can update backup with correct version for specific room",
			request: func(t *testing.T) *http.Request {
				reqBody := test.WithJSONBody(t, map[string]interface{}{
					"sessions": map[string]uapi.KeyBackupSession{
						"dummySession": {
							FirstMessageIndex: 1,
							IsVerified:        true,
							SessionData:       json.RawMessage("{}"),
						},
					},
				})
				req := test.NewRequest(t, http.MethodPut, "/_matrix/client/v3/room_keys/keys/!testroom:test", reqBody, test.WithQueryParams(map[string]string{
					"version": "1",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
				t.Logf("%#v", rec.Body.String())
			},
		},
		{
			name: "can update backup with correct version for specific room and session",
			request: func(t *testing.T) *http.Request {
				reqBody := test.WithJSONBody(t, uapi.KeyBackupSession{
					FirstMessageIndex: 1,
					SessionData:       json.RawMessage("{}"),
					IsVerified:        true,
					ForwardedCount:    0,
				})
				req := test.NewRequest(t, http.MethodPut, "/_matrix/client/v3/room_keys/keys/!testroom:test/dummySession", reqBody, test.WithQueryParams(map[string]string{
					"version": "1",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
			},
		},
		{
			name: "can update backup by version",
			request: func(t *testing.T) *http.Request {
				reqBody := test.WithJSONBody(t, uapi.KeyBackupSession{
					FirstMessageIndex: 1,
					SessionData:       json.RawMessage("{}"),
					IsVerified:        true,
					ForwardedCount:    0,
				})
				req := test.NewRequest(t, http.MethodPut, "/_matrix/client/v3/room_keys/version/1", reqBody, test.WithQueryParams(map[string]string{"version": "1"}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
				t.Logf("%#v", rec.Body.String())
			},
		},
		{
			name: "can not update backup by version for invalid version",
			request: func(t *testing.T) *http.Request {
				reqBody := test.WithJSONBody(t, uapi.KeyBackupSession{
					FirstMessageIndex: 1,
					SessionData:       json.RawMessage("{}"),
					IsVerified:        true,
					ForwardedCount:    0,
				})
				req := test.NewRequest(t, http.MethodPut, "/_matrix/client/v3/room_keys/version/2", reqBody, test.WithQueryParams(map[string]string{"version": "1"}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
			},
		},
		{
			name: "can query backup sessions",
			request: func(t *testing.T) *http.Request {
				req := test.NewRequest(t, http.MethodGet, "/_matrix/client/v3/room_keys/keys", test.WithQueryParams(map[string]string{
					"version": "1",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
				if gotRooms := gjson.GetBytes(rec.Body.Bytes(), "rooms").Map(); len(gotRooms) != 1 {
					t.Fatalf("expected one room in response, but got %#v", rec.Body.String())
				}
			},
		},
		{
			name: "can query backup sessions by room",
			request: func(t *testing.T) *http.Request {
				req := test.NewRequest(t, http.MethodGet, "/_matrix/client/v3/room_keys/keys/!testroom:test", test.WithQueryParams(map[string]string{
					"version": "1",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
				if gotRooms := gjson.GetBytes(rec.Body.Bytes(), "sessions").Map(); len(gotRooms) != 1 {
					t.Fatalf("expected one session in response, but got %#v", rec.Body.String())
				}
			},
		},
		{
			name: "can query backup sessions by room and sessionID",
			request: func(t *testing.T) *http.Request {
				req := test.NewRequest(t, http.MethodGet, "/_matrix/client/v3/room_keys/keys/!testroom:test/dummySession", test.WithQueryParams(map[string]string{
					"version": "1",
				}))
				return req
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
				if !gjson.GetBytes(rec.Body.Bytes(), "is_verified").Bool() {
					t.Fatalf("expected session to be verified, but wasn't: %#v", rec.Body.String())
				}
			},
		},
		{
			name: "can not delete invalid version backup",
			request: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/room_keys/version/2", nil)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusNotFound)
			},
		},
		{
			name: "can delete version backup",
			request: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/room_keys/version/1", nil)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
			},
		},
		{
			name: "deleting the same backup version twice doesn't error",
			request: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/room_keys/version/1", nil)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusOK)
			},
		},
		{
			name: "deleting an empty version doesn't work", // make sure we can't delete an empty backup version. Handled at the router level
			request: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodDelete, "/_matrix/client/v3/room_keys/version/", nil)
			},
			validate: func(t *testing.T, rec *httptest.ResponseRecorder) {
				handleResponseCode(t, rec, http.StatusNotFound)
			},
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cfg.ClientAPI.RateLimiting.Enabled = false
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		natsInstance := jetstream.NATSInstance{}
		defer close()

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)

		// We mostly need the rsAPI for this test, so nil for other APIs/caches etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		accessTokens := map[*test.User]userDevice{
			alice: {},
		}
		createAccessTokens(t, accessTokens, userAPI, processCtx.Context(), routers)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				req := tc.request(t)
				req.Header.Set("Authorization", "Bearer "+accessTokens[alice].accessToken)
				routers.Client.ServeHTTP(rec, req)
				tc.validate(t, rec)
			})
		}
	})
}
