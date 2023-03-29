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
