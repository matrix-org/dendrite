package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
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
		base, baseClose := testrig.CreateBaseDendrite(t, dbType)
		defer baseClose()
		base.Cfg.ClientAPI.RateLimiting.Enabled = false
		// add a vhost
		base.Cfg.Global.VirtualHosts = append(base.Cfg.Global.VirtualHosts, &config.VirtualHost{
			SigningIdentity: gomatrixserverlib.SigningIdentity{ServerName: "vh1"},
		})

		caches := caching.NewRistrettoCache(base.Cfg.Global.Cache.EstimatedMaxSize, base.Cfg.Global.Cache.MaxAge, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(base, caches)
		// Needed for /login
		userAPI := userapi.NewInternalAPI(base, rsAPI, nil)

		// We mostly need the userAPI for this test, so nil for other APIs/caches etc.
		Setup(base, &base.Cfg.ClientAPI, nil, nil, userAPI, nil, nil, nil, nil, nil, nil, &base.Cfg.MSCs, nil)

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
				base.PublicClientAPIMux.ServeHTTP(rec, req)
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
