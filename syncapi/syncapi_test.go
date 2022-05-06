package syncapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/nats-io/nats.go"
)

type syncRoomserverAPI struct {
	rsapi.RoomserverInternalAPI
}

type syncUserAPI struct {
	userapi.SyncUserAPI
	accounts []userapi.Device
}

func (s *syncUserAPI) QueryAccessToken(ctx context.Context, req *userapi.QueryAccessTokenRequest, res *userapi.QueryAccessTokenResponse) error {
	for _, acc := range s.accounts {
		if acc.AccessToken == req.AccessToken {
			res.Device = &acc
			return nil
		}
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
		testSync(t, dbType)
	})
}

func testSync(t *testing.T, dbType test.DBType) {
	user := test.NewUser()
	room := test.NewRoom(t, user)
	alice := userapi.Device{
		ID:          "ALICEID",
		UserID:      user.ID,
		AccessToken: "ALICE_BEARER_TOKEN",
		DisplayName: "Alice",
		AccountType: userapi.AccountTypeUser,
	}

	base, close := test.CreateBaseDendrite(t, dbType)
	defer close()

	jsctx, _ := jetstream.Prepare(base.ProcessContext, &base.Cfg.Global.JetStream)
	defer jetstream.DeleteAllStreams(jsctx, &base.Cfg.Global.JetStream)
	var msgs []*nats.Msg
	for _, ev := range room.Events() {
		msgs = append(msgs, test.NewOutputEventMsg(t, base, room.ID, api.OutputEvent{
			Type: rsapi.OutputTypeNewRoomEvent,
			NewRoomEvent: &rsapi.OutputNewRoomEvent{
				Event: ev,
			},
		}))
	}
	test.MustPublishMsgs(t, jsctx, msgs...)

	AddPublicRoutes(base, &syncUserAPI{accounts: []userapi.Device{alice}}, &syncRoomserverAPI{}, &syncKeyAPI{})

	testCases := []struct {
		name            string
		req             *http.Request
		wantCode        int
		wantJoinedRooms []string
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
				"access_token": alice.AccessToken,
				"timeout":      "0",
			})),
			wantCode:        200,
			wantJoinedRooms: []string{room.ID},
		},
	}
	// TODO: find a better way
	time.Sleep(100 * time.Millisecond)

	for _, tc := range testCases {
		w := httptest.NewRecorder()
		base.PublicClientAPIMux.ServeHTTP(w, tc.req)
		if w.Code != tc.wantCode {
			t.Fatalf("%s: got HTTP %d want %d", tc.name, w.Code, tc.wantCode)
		}
		/*
			if tc.wantJoinedRooms != nil {
				var res types.Response
				if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
					t.Fatalf("%s: failed to decode response body: %s", tc.name, err)
				}
				if len(res.Rooms.Join) != len(tc.wantJoinedRooms) {
					t.Errorf("%s: got %v joined rooms, want %v.\nResponse: %+v", tc.name, len(res.Rooms.Join), len(tc.wantJoinedRooms), res)
				}
			} */
	}
}
