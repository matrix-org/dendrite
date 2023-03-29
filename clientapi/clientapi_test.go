package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

func TestCapabilities(t *testing.T) {
	alice := test.NewUser(t)
	ctx := context.Background()

	// construct the expected result
	versionsMap := map[gomatrixserverlib.RoomVersion]string{}
	for v, desc := range version.SupportedRoomVersions() {
		if desc.Stable {
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
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
		// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]string{
			alice: "",
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
				tc.request.Header.Set("Authorization", "Bearer "+accessTokens[alice])
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
	userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
	//rsAPI.SetUserAPI(userAPI)
	// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
	AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

	// Create the users in the userapi and login
	accessTokens := map[*test.User]string{
		alice: "",
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
			req.Header.Set("Authorization", "Bearer "+accessTokens[alice])
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
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
		// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]string{
			alice: "",
		}
		createAccessTokens(t, accessTokens, userAPI, ctx, routers)

		identityServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.String(), "getValidated3pid"):
				resp := threepid.GetValidatedResponse{}
				switch r.URL.Query().Get("client_secret") {
				case "fail":
					resp.ErrCode = "M_SESSION_NOT_VALIDATED"
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
				tc.request.Header.Set("Authorization", "Bearer "+accessTokens[alice])

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
