package routing

import (
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
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
	uapi "github.com/matrix-org/dendrite/userapi/api"
)

func TestLogin(t *testing.T) {
	aliceAdmin := test.NewUser(t, test.WithAccountType(uapi.AccountTypeAdmin))
	bobUser := &test.User{ID: "@bob:test", AccountType: uapi.AccountTypeUser}
	charlie := &test.User{ID: "@Charlie:test", AccountType: uapi.AccountTypeUser}
	vhUser := &test.User{ID: "@vhuser:vh1"}

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()
		cfg.ClientAPI.RateLimiting.Enabled = false
		natsInstance := jetstream.NATSInstance{}
		// add a vhost
		cfg.Global.VirtualHosts = append(cfg.Global.VirtualHosts, &config.VirtualHost{
			SigningIdentity: fclient.SigningIdentity{ServerName: "vh1"},
		})

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		routers := httputil.NewRouters()
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		// Needed for /login
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)

		// We mostly need the userAPI for this test, so nil for other APIs/caches etc.
		Setup(routers, cfg, nil, nil, userAPI, nil, nil, nil, nil, nil, nil, nil, caching.DisableMetrics)

		// Create password
		password := util.RandomString(8)

		// create the users
		for _, u := range []*test.User{aliceAdmin, bobUser, vhUser, charlie} {
			localpart, serverName, _ := gomatrixserverlib.SplitID('@', u.ID)
			userRes := &uapi.PerformAccountCreationResponse{}

			if err := userAPI.PerformAccountCreation(ctx, &uapi.PerformAccountCreationRequest{
				AccountType: u.AccountType,
				Localpart:   localpart,
				ServerName:  serverName,
				Password:    password,
			}, userRes); err != nil {
				t.Errorf("failed to create account: %s", err)
			}
			if !userRes.AccountCreated {
				t.Fatalf("account not created")
			}
		}

		testCases := []struct {
			name   string
			userID string
			wantOK bool
		}{
			{
				name:   "aliceAdmin can login",
				userID: aliceAdmin.ID,
				wantOK: true,
			},
			{
				name:   "bobUser can login",
				userID: bobUser.ID,
				wantOK: true,
			},
			{
				name:   "vhuser can login",
				userID: vhUser.ID,
				wantOK: true,
			},
			{
				name:   "bob with uppercase can login",
				userID: "@Bob:test",
				wantOK: true,
			},
			{
				name:   "Charlie can login (existing uppercase)",
				userID: charlie.ID,
				wantOK: true,
			},
			{
				name:   "Charlie can not login with lowercase userID",
				userID: strings.ToLower(charlie.ID),
				wantOK: false,
			},
		}

		ctx := context.Background()

		t.Run("Supported log-in flows are returned", func(t *testing.T) {
			req := test.NewRequest(t, http.MethodGet, "/_matrix/client/v3/login")
			rec := httptest.NewRecorder()
			routers.Client.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("failed to get log-in flows: %s", rec.Body.String())
			}

			t.Logf("response: %s", rec.Body.String())
			resp := flows{}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}

			appServiceFound := false
			passwordFound := false
			for _, flow := range resp.Flows {
				if flow.Type == "m.login.password" {
					passwordFound = true
				} else if flow.Type == "m.login.application_service" {
					appServiceFound = true
				} else {
					t.Fatalf("got unknown login flow: %s", flow.Type)
				}
			}
			if !appServiceFound {
				t.Fatal("m.login.application_service missing from login flows")
			}
			if !passwordFound {
				t.Fatal("m.login.password missing from login flows")
			}
		})

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := test.NewRequest(t, http.MethodPost, "/_matrix/client/v3/login", test.WithJSONBody(t, map[string]interface{}{
					"type": authtypes.LoginTypePassword,
					"identifier": map[string]interface{}{
						"type": "m.id.user",
						"user": tc.userID,
					},
					"password": password,
				}))
				rec := httptest.NewRecorder()
				routers.Client.ServeHTTP(rec, req)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("failed to login: %s", rec.Body.String())
				}

				t.Logf("Response: %s", rec.Body.String())
				// get the response
				resp := loginResponse{}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				// everything OK
				if !tc.wantOK && resp.AccessToken == "" {
					return
				}
				if tc.wantOK && resp.AccessToken == "" {
					t.Fatalf("expected accessToken after successful login but got none: %+v", resp)
				}

				devicesResp := &uapi.QueryDevicesResponse{}
				if err := userAPI.QueryDevices(ctx, &uapi.QueryDevicesRequest{UserID: resp.UserID}, devicesResp); err != nil {
					t.Fatal(err)
				}
				for _, dev := range devicesResp.Devices {
					// We expect the userID on the device to be the same as resp.UserID
					if dev.UserID != resp.UserID {
						t.Fatalf("unexpected userID on device: %s", dev.UserID)
					}
				}
			})
		}
	})
}
