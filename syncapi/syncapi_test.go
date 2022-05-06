package syncapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/test"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

var (
	alice            = "@alice:localhost"
	aliceAccessToken = "ALICE_BEARER_TOKEN"
)

type syncRoomserverAPI struct {
	rsapi.RoomserverInternalAPI
}

type syncUserAPI struct {
	userapi.UserInternalAPI
}

func (s *syncUserAPI) QueryAccessToken(ctx context.Context, req *userapi.QueryAccessTokenRequest, res *userapi.QueryAccessTokenResponse) error {
	if req.AccessToken == aliceAccessToken {
		res.Device = &userapi.Device{
			ID:          "ID",
			UserID:      alice,
			AccessToken: aliceAccessToken,
			AccountType: userapi.AccountTypeUser,
			DisplayName: "Alice",
		}
		return nil
	}
	res.Err = "unknown user"
	return nil
}

func (s *syncUserAPI) PerformLastSeenUpdate(ctx context.Context, req *userapi.PerformLastSeenUpdateRequest, res *userapi.PerformLastSeenUpdateResponse) error {
	return nil
}

type syncKeyAPI struct {
	keyapi.KeyInternalAPI
}

func TestSyncAPI(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, close := test.CreateBaseDendrite(t, dbType)
		defer close()
		AddPublicRoutes(base, &syncUserAPI{}, &syncRoomserverAPI{}, &syncKeyAPI{})

		testCases := []struct {
			name     string
			req      *http.Request
			wantCode int
		}{
			{
				name: "missing access token",
				req: test.NewRequest(t, "GET", "/_matrix/client/v3/sync", test.WithQueryParams(map[string]string{
					"timeout": "0",
				})),
				wantCode: 401,
			},
			{
				name: "unknown access token",
				req: test.NewRequest(t, "GET", "/_matrix/client/v3/sync", test.WithQueryParams(map[string]string{
					"access_token": "foo",
					"timeout":      "0",
				})),
				wantCode: 401,
			},
			{
				name: "valid access token",
				req: test.NewRequest(t, "GET", "/_matrix/client/v3/sync", test.WithQueryParams(map[string]string{
					"access_token": aliceAccessToken,
					"timeout":      "0",
				})),
				wantCode: 200,
			},
		}

		for _, tc := range testCases {
			w := httptest.NewRecorder()
			base.PublicClientAPIMux.ServeHTTP(w, tc.req)
			if w.Code != tc.wantCode {
				t.Fatalf("%s: got HTTP %d want %d", tc.name, w.Code, tc.wantCode)
			}
		}
	})
}
