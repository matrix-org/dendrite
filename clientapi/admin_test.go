package clientapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"

	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
	uapi "github.com/matrix-org/dendrite/userapi/api"
)

func TestAdminResetPassword(t *testing.T) {
	alice := test.NewUser(t, test.WithAccountType(uapi.AccountTypeAdmin))
	bob := test.NewUser(t, test.WithAccountType(uapi.AccountTypeUser))
	vhUser := &test.User{ID: "@vhuser:vh1"}

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, baseClose := testrig.CreateBaseDendrite(t, dbType)
		defer baseClose()

		// add a vhost
		base.Cfg.Global.VirtualHosts = append(base.Cfg.Global.VirtualHosts, &config.VirtualHost{
			SigningIdentity: gomatrixserverlib.SigningIdentity{ServerName: "vh1"},
		})

		rsAPI := roomserver.NewInternalAPI(base)
		keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, nil, rsAPI)
		userAPI := userapi.NewInternalAPI(base, &base.Cfg.UserAPI, nil, keyAPI, rsAPI, nil)
		keyAPI.SetUserAPI(userAPI)
		AddPublicRoutes(base, nil, nil, nil, nil, nil, userAPI, nil, nil, nil)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]string{
			alice:  "",
			bob:    "",
			vhUser: "",
		}
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
			base.PublicClientAPIMux.ServeHTTP(rec, req)
			t.Logf("%+v\n", rec.Body.String())
			if rec.Code != http.StatusOK {
				t.Fatalf("failed to login: %s", rec.Body.String())
			}
			accessTokens[u] = gjson.GetBytes(rec.Body.Bytes(), "access_token").String()
		}

		testCases := []struct {
			name           string
			requestingUser *test.User
			userID         string
			requestOpt     test.HTTPRequestOpt
			wantOK         bool
			withHeader     bool
		}{
			{name: "Missing auth", requestingUser: bob, wantOK: false, userID: bob.ID},
			{name: "Bob is denied access", requestingUser: bob, wantOK: false, withHeader: true, userID: bob.ID},
			{name: "Alice is allowed access", requestingUser: alice, wantOK: true, withHeader: true, userID: bob.ID, requestOpt: test.WithJSONBody(t, map[string]interface{}{
				"password": "newPass",
			})},
			{name: "Alice is allowed access, missing userID", requestingUser: alice, wantOK: false, withHeader: true, userID: ""}, // this 404s
			{name: "Alice is allowed access, empty password", requestingUser: alice, wantOK: false, withHeader: true, userID: bob.ID, requestOpt: test.WithJSONBody(t, map[string]interface{}{
				"password": "",
			})},
			{name: "Alice is allowed access, unknown server name", requestingUser: alice, wantOK: false, withHeader: true, userID: "@doesnotexist:localhost", requestOpt: test.WithJSONBody(t, map[string]interface{}{})},
			{name: "Alice is allowed access, unknown user", requestingUser: alice, wantOK: false, withHeader: true, userID: "@doesnotexist:test", requestOpt: test.WithJSONBody(t, map[string]interface{}{})},
			{name: "Alice is allowed access, different vhost", requestingUser: alice, wantOK: true, withHeader: true, userID: vhUser.ID, requestOpt: test.WithJSONBody(t, map[string]interface{}{
				"password": "newPass",
			})},
			{name: "Alice is allowed access, existing user, missing body", requestingUser: alice, wantOK: false, withHeader: true, userID: bob.ID},
			{name: "Alice is allowed access, invalid userID", requestingUser: alice, wantOK: false, withHeader: true, userID: "!notauserid:test", requestOpt: test.WithJSONBody(t, map[string]interface{}{})},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := test.NewRequest(t, http.MethodPost, "/_dendrite/admin/resetPassword/"+tc.userID)
				if tc.requestOpt != nil {
					req = test.NewRequest(t, http.MethodPost, "/_dendrite/admin/resetPassword/"+tc.userID, tc.requestOpt)
				}

				if tc.withHeader {
					req.Header.Set("Authorization", "Bearer "+accessTokens[tc.requestingUser])
				}

				rec := httptest.NewRecorder()
				base.DendriteAdminMux.ServeHTTP(rec, req)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected http status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
				}
			})
		}
	})
}
