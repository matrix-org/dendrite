// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package userapi_test

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/bcrypt"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/dendrite/userapi/inthttp"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/internal"
	"github.com/matrix-org/dendrite/userapi/storage"
)

const (
	serverName = gomatrixserverlib.ServerName("example.com")
)

type apiTestOpts struct {
	loginTokenLifetime time.Duration
}

func MustMakeInternalAPI(t *testing.T, opts apiTestOpts, dbType test.DBType) (api.UserInternalAPI, storage.Database, func()) {
	if opts.loginTokenLifetime == 0 {
		opts.loginTokenLifetime = api.DefaultLoginTokenLifetime * time.Millisecond
	}
	base, baseclose := testrig.CreateBaseDendrite(t, dbType)
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	accountDB, err := storage.NewUserAPIDatabase(base, &config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, serverName, bcrypt.MinCost, config.DefaultOpenIDTokenLifetimeMS, opts.loginTokenLifetime, "")
	if err != nil {
		t.Fatalf("failed to create account DB: %s", err)
	}

	cfg := &config.UserAPI{
		Matrix: &config.Global{
			SigningIdentity: gomatrixserverlib.SigningIdentity{
				ServerName: serverName,
			},
		},
	}

	return &internal.UserInternalAPI{
			DB:     accountDB,
			Config: cfg,
		}, accountDB, func() {
			close()
			baseclose()
		}
}

func TestQueryProfile(t *testing.T) {
	aliceAvatarURL := "mxc://example.com/alice"
	aliceDisplayName := "Alice"
	// only one DBType, since userapi.AddInternalRoutes complains about multiple prometheus counters added
	userAPI, accountDB, close := MustMakeInternalAPI(t, apiTestOpts{}, test.DBTypeSQLite)
	defer close()
	_, err := accountDB.CreateAccount(context.TODO(), "alice", serverName, "foobar", "", api.AccountTypeUser)
	if err != nil {
		t.Fatalf("failed to make account: %s", err)
	}
	if _, _, err := accountDB.SetAvatarURL(context.TODO(), "alice", serverName, aliceAvatarURL); err != nil {
		t.Fatalf("failed to set avatar url: %s", err)
	}
	if _, _, err := accountDB.SetDisplayName(context.TODO(), "alice", serverName, aliceDisplayName); err != nil {
		t.Fatalf("failed to set display name: %s", err)
	}

	testCases := []struct {
		req     api.QueryProfileRequest
		wantRes api.QueryProfileResponse
		wantErr error
	}{
		{
			req: api.QueryProfileRequest{
				UserID: fmt.Sprintf("@alice:%s", serverName),
			},
			wantRes: api.QueryProfileResponse{
				UserExists:  true,
				AvatarURL:   aliceAvatarURL,
				DisplayName: aliceDisplayName,
			},
		},
		{
			req: api.QueryProfileRequest{
				UserID: fmt.Sprintf("@bob:%s", serverName),
			},
			wantRes: api.QueryProfileResponse{
				UserExists: false,
			},
		},
		{
			req: api.QueryProfileRequest{
				UserID: "@alice:wrongdomain.com",
			},
			wantErr: fmt.Errorf("wrong domain"),
		},
	}

	runCases := func(testAPI api.UserInternalAPI, http bool) {
		mode := "monolith"
		if http {
			mode = "HTTP"
		}
		for _, tc := range testCases {
			var gotRes api.QueryProfileResponse
			gotErr := testAPI.QueryProfile(context.TODO(), &tc.req, &gotRes)
			if tc.wantErr == nil && gotErr != nil || tc.wantErr != nil && gotErr == nil {
				t.Errorf("QueryProfile %s error, got %s want %s", mode, gotErr, tc.wantErr)
				continue
			}
			if !reflect.DeepEqual(tc.wantRes, gotRes) {
				t.Errorf("QueryProfile %s response got %+v want %+v", mode, gotRes, tc.wantRes)
			}
		}
	}

	t.Run("HTTP API", func(t *testing.T) {
		router := mux.NewRouter().PathPrefix(httputil.InternalPathPrefix).Subrouter()
		userapi.AddInternalRoutes(router, userAPI)
		apiURL, cancel := test.ListenAndServe(t, router, false)
		defer cancel()
		httpAPI, err := inthttp.NewUserAPIClient(apiURL, &http.Client{})
		if err != nil {
			t.Fatalf("failed to create HTTP client")
		}
		runCases(httpAPI, true)
	})
	t.Run("Monolith", func(t *testing.T) {
		runCases(userAPI, false)
	})
}

// TestPasswordlessLoginFails ensures that a passwordless account cannot
// be logged into using an arbitrary password (effectively a regression test
// for https://github.com/matrix-org/dendrite/issues/2780).
func TestPasswordlessLoginFails(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		userAPI, accountDB, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
		defer close()
		_, err := accountDB.CreateAccount(ctx, "auser", serverName, "", "", api.AccountTypeAppService)
		if err != nil {
			t.Fatalf("failed to make account: %s", err)
		}

		userReq := &api.QueryAccountByPasswordRequest{
			Localpart:         "auser",
			PlaintextPassword: "apassword",
		}
		userRes := &api.QueryAccountByPasswordResponse{}
		if err := userAPI.QueryAccountByPassword(ctx, userReq, userRes); err != nil {
			t.Fatal(err)
		}
		if userRes.Exists || userRes.Account != nil {
			t.Fatal("QueryAccountByPassword should not return correctly for a passwordless account")
		}
	})
}

func TestLoginToken(t *testing.T) {
	ctx := context.Background()

	t.Run("tokenLoginFlow", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			userAPI, accountDB, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
			defer close()
			_, err := accountDB.CreateAccount(ctx, "auser", serverName, "apassword", "", api.AccountTypeUser)
			if err != nil {
				t.Fatalf("failed to make account: %s", err)
			}

			t.Log("Creating a login token like the SSO callback would...")

			creq := api.PerformLoginTokenCreationRequest{
				Data: api.LoginTokenData{UserID: "@auser:example.com"},
			}
			var cresp api.PerformLoginTokenCreationResponse
			if err := userAPI.PerformLoginTokenCreation(ctx, &creq, &cresp); err != nil {
				t.Fatalf("PerformLoginTokenCreation failed: %v", err)
			}

			if cresp.Metadata.Token == "" {
				t.Errorf("PerformLoginTokenCreation Token: got %q, want non-empty", cresp.Metadata.Token)
			}
			if cresp.Metadata.Expiration.Before(time.Now()) {
				t.Errorf("PerformLoginTokenCreation Expiration: got %v, want non-expired", cresp.Metadata.Expiration)
			}

			t.Log("Querying the login token like /login with m.login.token would...")

			qreq := api.QueryLoginTokenRequest{Token: cresp.Metadata.Token}
			var qresp api.QueryLoginTokenResponse
			if err := userAPI.QueryLoginToken(ctx, &qreq, &qresp); err != nil {
				t.Fatalf("QueryLoginToken failed: %v", err)
			}

			if qresp.Data == nil {
				t.Errorf("QueryLoginToken Data: got %v, want non-nil", qresp.Data)
			} else if want := "@auser:example.com"; qresp.Data.UserID != want {
				t.Errorf("QueryLoginToken UserID: got %q, want %q", qresp.Data.UserID, want)
			}

			t.Log("Deleting the login token like /login with m.login.token would...")

			dreq := api.PerformLoginTokenDeletionRequest{Token: cresp.Metadata.Token}
			var dresp api.PerformLoginTokenDeletionResponse
			if err := userAPI.PerformLoginTokenDeletion(ctx, &dreq, &dresp); err != nil {
				t.Fatalf("PerformLoginTokenDeletion failed: %v", err)
			}
		})
	})

	t.Run("expiredTokenIsNotReturned", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			userAPI, _, close := MustMakeInternalAPI(t, apiTestOpts{loginTokenLifetime: -1 * time.Second}, dbType)
			defer close()

			creq := api.PerformLoginTokenCreationRequest{
				Data: api.LoginTokenData{UserID: "@auser:example.com"},
			}
			var cresp api.PerformLoginTokenCreationResponse
			if err := userAPI.PerformLoginTokenCreation(ctx, &creq, &cresp); err != nil {
				t.Fatalf("PerformLoginTokenCreation failed: %v", err)
			}

			qreq := api.QueryLoginTokenRequest{Token: cresp.Metadata.Token}
			var qresp api.QueryLoginTokenResponse
			if err := userAPI.QueryLoginToken(ctx, &qreq, &qresp); err != nil {
				t.Fatalf("QueryLoginToken failed: %v", err)
			}

			if qresp.Data != nil {
				t.Errorf("QueryLoginToken Data: got %v, want nil", qresp.Data)
			}
		})
	})

	t.Run("deleteWorks", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			userAPI, _, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
			defer close()

			creq := api.PerformLoginTokenCreationRequest{
				Data: api.LoginTokenData{UserID: "@auser:example.com"},
			}
			var cresp api.PerformLoginTokenCreationResponse
			if err := userAPI.PerformLoginTokenCreation(ctx, &creq, &cresp); err != nil {
				t.Fatalf("PerformLoginTokenCreation failed: %v", err)
			}

			dreq := api.PerformLoginTokenDeletionRequest{Token: cresp.Metadata.Token}
			var dresp api.PerformLoginTokenDeletionResponse
			if err := userAPI.PerformLoginTokenDeletion(ctx, &dreq, &dresp); err != nil {
				t.Fatalf("PerformLoginTokenDeletion failed: %v", err)
			}

			qreq := api.QueryLoginTokenRequest{Token: cresp.Metadata.Token}
			var qresp api.QueryLoginTokenResponse
			if err := userAPI.QueryLoginToken(ctx, &qreq, &qresp); err != nil {
				t.Fatalf("QueryLoginToken failed: %v", err)
			}

			if qresp.Data != nil {
				t.Errorf("QueryLoginToken Data: got %v, want nil", qresp.Data)
			}
		})
	})

	t.Run("deleteUnknownIsNoOp", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			userAPI, _, close := MustMakeInternalAPI(t, apiTestOpts{}, dbType)
			defer close()
			dreq := api.PerformLoginTokenDeletionRequest{Token: "non-existent token"}
			var dresp api.PerformLoginTokenDeletionResponse
			if err := userAPI.PerformLoginTokenDeletion(ctx, &dreq, &dresp); err != nil {
				t.Fatalf("PerformLoginTokenDeletion failed: %v", err)
			}
		})
	})
}
