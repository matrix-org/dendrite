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

package userapi

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
	"github.com/matrix-org/dendrite/internal/test"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/internal"
	"github.com/matrix-org/dendrite/userapi/inthttp"
	"github.com/matrix-org/dendrite/userapi/storage"
)

const (
	serverName = gomatrixserverlib.ServerName("example.com")
)

type apiTestOpts struct {
	loginTokenLifetime time.Duration
}

func MustMakeInternalAPI(t *testing.T, opts apiTestOpts) (api.UserInternalAPI, storage.Database) {
	if opts.loginTokenLifetime == 0 {
		opts.loginTokenLifetime = api.DefaultLoginTokenLifetime * time.Millisecond
	}
	dbopts := &config.DatabaseOptions{
		ConnectionString:   "file::memory:",
		MaxOpenConnections: 1,
		MaxIdleConnections: 1,
	}
	accountDB, err := storage.NewDatabase(dbopts, serverName, bcrypt.MinCost, config.DefaultOpenIDTokenLifetimeMS, opts.loginTokenLifetime, "")
	if err != nil {
		t.Fatalf("failed to create account DB: %s", err)
	}

	cfg := &config.UserAPI{
		Matrix: &config.Global{
			ServerName: serverName,
		},
	}

	return &internal.UserInternalAPI{
		DB:         accountDB,
		ServerName: cfg.Matrix.ServerName,
	}, accountDB
}

func TestQueryProfile(t *testing.T) {
	aliceAvatarURL := "mxc://example.com/alice"
	aliceDisplayName := "Alice"
	userAPI, accountDB := MustMakeInternalAPI(t, apiTestOpts{})
	_, err := accountDB.CreateAccount(context.TODO(), "alice", "foobar", "", api.AccountTypeUser)
	if err != nil {
		t.Fatalf("failed to make account: %s", err)
	}
	if err := accountDB.SetAvatarURL(context.TODO(), "alice", aliceAvatarURL); err != nil {
		t.Fatalf("failed to set avatar url: %s", err)
	}
	if err := accountDB.SetDisplayName(context.TODO(), "alice", aliceDisplayName); err != nil {
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

	runCases := func(testAPI api.UserInternalAPI) {
		for _, tc := range testCases {
			var gotRes api.QueryProfileResponse
			gotErr := testAPI.QueryProfile(context.TODO(), &tc.req, &gotRes)
			if tc.wantErr == nil && gotErr != nil || tc.wantErr != nil && gotErr == nil {
				t.Errorf("QueryProfile error, got %s want %s", gotErr, tc.wantErr)
				continue
			}
			if !reflect.DeepEqual(tc.wantRes, gotRes) {
				t.Errorf("QueryProfile response got %+v want %+v", gotRes, tc.wantRes)
			}
		}
	}

	t.Run("HTTP API", func(t *testing.T) {
		router := mux.NewRouter().PathPrefix(httputil.InternalPathPrefix).Subrouter()
		AddInternalRoutes(router, userAPI)
		apiURL, cancel := test.ListenAndServe(t, router, false)
		defer cancel()
		httpAPI, err := inthttp.NewUserAPIClient(apiURL, &http.Client{})
		if err != nil {
			t.Fatalf("failed to create HTTP client")
		}
		runCases(httpAPI)
	})
	t.Run("Monolith", func(t *testing.T) {
		runCases(userAPI)
	})
}

func TestLoginToken(t *testing.T) {
	ctx := context.Background()

	t.Run("tokenLoginFlow", func(t *testing.T) {
		userAPI, accountDB := MustMakeInternalAPI(t, apiTestOpts{})

		_, err := accountDB.CreateAccount(ctx, "auser", "apassword", "", api.AccountTypeUser)
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

	t.Run("expiredTokenIsNotReturned", func(t *testing.T) {
		userAPI, _ := MustMakeInternalAPI(t, apiTestOpts{loginTokenLifetime: -1 * time.Second})

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

	t.Run("deleteWorks", func(t *testing.T) {
		userAPI, _ := MustMakeInternalAPI(t, apiTestOpts{})

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

	t.Run("deleteUnknownIsNoOp", func(t *testing.T) {
		userAPI, _ := MustMakeInternalAPI(t, apiTestOpts{})

		dreq := api.PerformLoginTokenDeletionRequest{Token: "non-existent token"}
		var dresp api.PerformLoginTokenDeletionResponse
		if err := userAPI.PerformLoginTokenDeletion(ctx, &dreq, &dresp); err != nil {
			t.Fatalf("PerformLoginTokenDeletion failed: %v", err)
		}
	})
}
