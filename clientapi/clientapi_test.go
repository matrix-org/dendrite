package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"
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
			name         string
			requestUser  *test.User
			deviceUser   *test.User
			request      *http.Request
			withDeviceID bool
			wantOK       bool
			validateFunc func(t *testing.T, device userDevice, routers httputil.Routers)
		}{
			{
				name:        "can get all devices",
				requestUser: alice,
				request:     httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/devices", strings.NewReader("")),
				wantOK:      true,
			},
			{
				name:         "can get specific own device",
				requestUser:  alice,
				deviceUser:   alice,
				request:      httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/devices/", strings.NewReader("")),
				withDeviceID: true,
				wantOK:       true,
			},
			{
				name:         "can not get device for different user",
				requestUser:  alice,
				deviceUser:   bob,
				request:      httptest.NewRequest(http.MethodGet, "/_matrix/client/v3/devices/", strings.NewReader("")),
				withDeviceID: true,
			},
			{
				name:         "can update own device",
				requestUser:  alice,
				deviceUser:   alice,
				request:      httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/devices/", strings.NewReader(`{"display_name":"my new displayname"}`)),
				withDeviceID: true,
				wantOK:       true,
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
				name:         "can not update device for different user",
				requestUser:  alice,
				deviceUser:   bob,
				request:      httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/devices/", strings.NewReader(`{"display_name":"my new displayname"}`)),
				withDeviceID: true,
			},
		}

		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		natsInstance := jetstream.NATSInstance{}
		defer close()

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
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
				if tc.withDeviceID {
					tc.request = httptest.NewRequest(tc.request.Method, tc.request.RequestURI+accessTokens[tc.deviceUser].deviceID, tc.request.Body)
				}
				tc.request.Header.Set("Authorization", "Bearer "+dev.accessToken)
				rec := httptest.NewRecorder()
				routers.Client.ServeHTTP(rec, tc.request)
				if tc.wantOK && rec.Code != 200 {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}
				if !tc.wantOK && rec.Code != 200 {
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
