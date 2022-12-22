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
	aliceAdmin := test.NewUser(t, test.WithAccountType(uapi.AccountTypeAdmin))
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
		// Needed for changing the password/login
		keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, nil, rsAPI)
		userAPI := userapi.NewInternalAPI(base, &base.Cfg.UserAPI, nil, keyAPI, rsAPI, nil)
		keyAPI.SetUserAPI(userAPI)
		// We mostly need the userAPI for this test, so nil for other APIs/caches etc.
		AddPublicRoutes(base, nil, nil, nil, nil, nil, userAPI, nil, nil, nil)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]string{
			aliceAdmin: "",
			bob:        "",
			vhUser:     "",
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
			{name: "Alice is allowed access", requestingUser: aliceAdmin, wantOK: true, withHeader: true, userID: bob.ID, requestOpt: test.WithJSONBody(t, map[string]interface{}{
				"password": util.RandomString(8),
			})},
			{name: "missing userID does not call function", requestingUser: aliceAdmin, wantOK: false, withHeader: true, userID: ""}, // this 404s
			{name: "rejects empty password", requestingUser: aliceAdmin, wantOK: false, withHeader: true, userID: bob.ID, requestOpt: test.WithJSONBody(t, map[string]interface{}{
				"password": "",
			})},
			{name: "rejects unknown server name", requestingUser: aliceAdmin, wantOK: false, withHeader: true, userID: "@doesnotexist:localhost", requestOpt: test.WithJSONBody(t, map[string]interface{}{})},
			{name: "rejects unknown user", requestingUser: aliceAdmin, wantOK: false, withHeader: true, userID: "@doesnotexist:test", requestOpt: test.WithJSONBody(t, map[string]interface{}{})},
			{name: "allows changing password for different vhost", requestingUser: aliceAdmin, wantOK: true, withHeader: true, userID: vhUser.ID, requestOpt: test.WithJSONBody(t, map[string]interface{}{
				"password": util.RandomString(8),
			})},
			{name: "rejects existing user, missing body", requestingUser: aliceAdmin, wantOK: false, withHeader: true, userID: bob.ID},
			{name: "rejects invalid userID", requestingUser: aliceAdmin, wantOK: false, withHeader: true, userID: "!notauserid:test", requestOpt: test.WithJSONBody(t, map[string]interface{}{})},
			{name: "rejects invalid json", requestingUser: aliceAdmin, wantOK: false, withHeader: true, userID: bob.ID, requestOpt: test.WithJSONBody(t, `{invalidJSON}`)},
			{name: "rejects too weak password", requestingUser: aliceAdmin, wantOK: false, withHeader: true, userID: bob.ID, requestOpt: test.WithJSONBody(t, map[string]interface{}{
				"password": util.RandomString(6),
			})},
			{name: "rejects too long password", requestingUser: aliceAdmin, wantOK: false, withHeader: true, userID: bob.ID, requestOpt: test.WithJSONBody(t, map[string]interface{}{
				"password": util.RandomString(513),
			})},
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
				t.Logf("%s", rec.Body.String())
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected http status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
				}
			})
		}
	})
}
