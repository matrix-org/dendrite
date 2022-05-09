package syncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/test"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/nats-io/nats.go"
)

type syncRoomserverAPI struct {
	rsapi.SyncRoomserverAPI
	rooms []*test.Room
}

func (s *syncRoomserverAPI) QueryEventsByID(ctx context.Context, req *rsapi.QueryEventsByIDRequest, res *rsapi.QueryEventsByIDResponse) error {
NextEvent:
	for _, eventID := range req.EventIDs {
		for _, r := range s.rooms {
			for _, ev := range r.Events() {
				fmt.Println(ev.EventID())
				if ev.EventID() == eventID {
					res.Events = append(res.Events, ev)
					continue NextEvent
				}
			}
		}
	}
	fmt.Println("QueryEventsByID", req.EventIDs, " returning ", len(res.Events))
	return nil
}

func (s *syncRoomserverAPI) QueryLatestEventsAndState(ctx context.Context, req *rsapi.QueryLatestEventsAndStateRequest, res *rsapi.QueryLatestEventsAndStateResponse) error {
	var room *test.Room
	for _, r := range s.rooms {
		if r.ID == req.RoomID {
			room = r
			break
		}
	}
	if room == nil {
		res.RoomExists = false
		return nil
	}
	res.RoomVersion = room.Version
	return nil // TODO: return state
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
	testSync(t, test.DBTypePostgres)
	/*
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			testSync(t, dbType)
		}) */
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
		var addsStateIDs []string
		if ev.StateKey() != nil {
			addsStateIDs = append(addsStateIDs, ev.EventID())
		}
		msgs = append(msgs, test.NewOutputEventMsg(t, base, room.ID, api.OutputEvent{
			Type: rsapi.OutputTypeNewRoomEvent,
			NewRoomEvent: &rsapi.OutputNewRoomEvent{
				Event:             ev,
				AddsStateEventIDs: addsStateIDs,
			},
		}))
	}
	test.MustPublishMsgs(t, jsctx, msgs...)

	AddPublicRoutes(base, &syncUserAPI{accounts: []userapi.Device{alice}}, &syncRoomserverAPI{rooms: []*test.Room{room}}, &syncKeyAPI{})

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
	time.Sleep(1000 * time.Millisecond)

	for _, tc := range testCases {
		w := httptest.NewRecorder()
		base.PublicClientAPIMux.ServeHTTP(w, tc.req)
		if w.Code != tc.wantCode {
			t.Fatalf("%s: got HTTP %d want %d", tc.name, w.Code, tc.wantCode)
		}
		if tc.wantJoinedRooms != nil {
			var res types.Response
			if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
				t.Fatalf("%s: failed to decode response body: %s", tc.name, err)
			}
			if len(res.Rooms.Join) != len(tc.wantJoinedRooms) {
				t.Errorf("%s: got %v joined rooms, want %v.\nResponse: %+v", tc.name, len(res.Rooms.Join), len(tc.wantJoinedRooms), res)
			}
		}
	}
}
