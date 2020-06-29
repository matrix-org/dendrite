package userapi_test

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/test"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/inthttp"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/dendrite/userapi/storage/devices"
	"github.com/matrix-org/gomatrixserverlib"
)

const (
	serverName = gomatrixserverlib.ServerName("example.com")
)

func MustMakeInternalAPI(t *testing.T) (api.UserInternalAPI, accounts.Database, devices.Database) {
	accountDB, err := accounts.NewDatabase("file::memory:", nil, serverName)
	if err != nil {
		t.Fatalf("failed to create account DB: %s", err)
	}
	deviceDB, err := devices.NewDatabase("file::memory:", nil, serverName)
	if err != nil {
		t.Fatalf("failed to create device DB: %s", err)
	}

	return userapi.NewInternalAPI(accountDB, deviceDB, serverName, nil), accountDB, deviceDB
}

func TestQueryProfile(t *testing.T) {
	aliceAvatarURL := "mxc://example.com/alice"
	aliceDisplayName := "Alice"
	userAPI, accountDB, _ := MustMakeInternalAPI(t)
	_, err := accountDB.CreateAccount(context.TODO(), "alice", "foobar", "")
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
		userapi.AddInternalRoutes(router, userAPI)
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
