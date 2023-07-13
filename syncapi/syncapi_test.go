package syncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"
	"github.com/tidwall/gjson"

	rstypes "github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/syncapi/routing"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/synctypes"

	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type syncRoomserverAPI struct {
	rsapi.SyncRoomserverAPI
	rooms []*test.Room
}

func (s *syncRoomserverAPI) QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	return spec.NewUserID(string(senderID), true)
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

func (s *syncRoomserverAPI) QuerySharedUsers(ctx context.Context, req *rsapi.QuerySharedUsersRequest, res *rsapi.QuerySharedUsersResponse) error {
	res.UserIDsToCount = make(map[string]int)
	return nil
}
func (s *syncRoomserverAPI) QueryBulkStateContent(ctx context.Context, req *rsapi.QueryBulkStateContentRequest, res *rsapi.QueryBulkStateContentResponse) error {
	return nil
}

func (s *syncRoomserverAPI) QueryMembershipForUser(ctx context.Context, req *rsapi.QueryMembershipForUserRequest, res *rsapi.QueryMembershipForUserResponse) error {
	res.IsRoomForgotten = false
	res.RoomExists = true
	return nil
}

func (s *syncRoomserverAPI) QueryMembershipAtEvent(ctx context.Context, req *rsapi.QueryMembershipAtEventRequest, res *rsapi.QueryMembershipAtEventResponse) error {
	return nil
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

func (s *syncUserAPI) QueryKeyChanges(ctx context.Context, req *userapi.QueryKeyChangesRequest, res *userapi.QueryKeyChangesResponse) error {
	return nil
}

func (s *syncUserAPI) QueryOneTimeKeys(ctx context.Context, req *userapi.QueryOneTimeKeysRequest, res *userapi.QueryOneTimeKeysResponse) error {
	return nil
}

func (s *syncUserAPI) PerformLastSeenUpdate(ctx context.Context, req *userapi.PerformLastSeenUpdateRequest, res *userapi.PerformLastSeenUpdateResponse) error {
	return nil
}

func TestSyncAPIAccessTokens(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		testSyncAccessTokens(t, dbType)
	})
}

func testSyncAccessTokens(t *testing.T, dbType test.DBType) {
	user := test.NewUser(t)
	room := test.NewRoom(t, user)
	alice := userapi.Device{
		ID:          "ALICEID",
		UserID:      user.ID,
		AccessToken: "ALICE_BEARER_TOKEN",
		DisplayName: "Alice",
		AccountType: userapi.AccountTypeUser,
	}

	cfg, processCtx, close := testrig.CreateConfig(t, dbType)
	routers := httputil.NewRouters()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
	natsInstance := jetstream.NATSInstance{}
	defer close()

	jsctx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
	defer jetstream.DeleteAllStreams(jsctx, &cfg.Global.JetStream)
	msgs := toNATSMsgs(t, cfg, room.Events()...)
	AddPublicRoutes(processCtx, routers, cfg, cm, &natsInstance, &syncUserAPI{accounts: []userapi.Device{alice}}, &syncRoomserverAPI{rooms: []*test.Room{room}}, caches, caching.DisableMetrics)
	testrig.MustPublishMsgs(t, jsctx, msgs...)

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

	syncUntil(t, routers, alice.AccessToken, false, func(syncBody string) bool {
		// wait for the last sent eventID to come down sync
		path := fmt.Sprintf(`rooms.join.%s.timeline.events.#(event_id=="%s")`, room.ID, room.Events()[len(room.Events())-1].EventID())
		return gjson.Get(syncBody, path).Exists()
	})

	for _, tc := range testCases {
		w := httptest.NewRecorder()
		routers.Client.ServeHTTP(w, tc.req)
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
			t.Logf("res: %+v", res.Rooms.Join[room.ID])

			gotEventIDs := make([]string, len(res.Rooms.Join[room.ID].Timeline.Events))
			for i, ev := range res.Rooms.Join[room.ID].Timeline.Events {
				gotEventIDs[i] = ev.EventID
			}
			test.AssertEventIDsEqual(t, gotEventIDs, room.Events())
		}
	}
}

// Tests what happens when we create a room and then /sync before all events from /createRoom have
// been sent to the syncapi
func TestSyncAPICreateRoomSyncEarly(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		testSyncAPICreateRoomSyncEarly(t, dbType)
	})
}

func testSyncAPICreateRoomSyncEarly(t *testing.T, dbType test.DBType) {
	t.Skip("Skipped, possibly fixed")
	user := test.NewUser(t)
	room := test.NewRoom(t, user)
	alice := userapi.Device{
		ID:          "ALICEID",
		UserID:      user.ID,
		AccessToken: "ALICE_BEARER_TOKEN",
		DisplayName: "Alice",
		AccountType: userapi.AccountTypeUser,
	}

	cfg, processCtx, close := testrig.CreateConfig(t, dbType)
	routers := httputil.NewRouters()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
	defer close()
	natsInstance := jetstream.NATSInstance{}

	jsctx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
	defer jetstream.DeleteAllStreams(jsctx, &cfg.Global.JetStream)
	// order is:
	// m.room.create
	// m.room.member
	// m.room.power_levels
	// m.room.join_rules
	// m.room.history_visibility
	msgs := toNATSMsgs(t, cfg, room.Events()...)
	sinceTokens := make([]string, len(msgs))
	AddPublicRoutes(processCtx, routers, cfg, cm, &natsInstance, &syncUserAPI{accounts: []userapi.Device{alice}}, &syncRoomserverAPI{rooms: []*test.Room{room}}, caches, caching.DisableMetrics)
	for i, msg := range msgs {
		testrig.MustPublishMsgs(t, jsctx, msg)
		time.Sleep(100 * time.Millisecond)
		w := httptest.NewRecorder()
		routers.Client.ServeHTTP(w, test.NewRequest(t, "GET", "/_matrix/client/v3/sync", test.WithQueryParams(map[string]string{
			"access_token": alice.AccessToken,
			"timeout":      "0",
		})))
		if w.Code != 200 {
			t.Errorf("got HTTP %d want 200", w.Code)
			continue
		}
		var res types.Response
		if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
			t.Errorf("failed to decode response body: %s", err)
		}
		sinceTokens[i] = res.NextBatch.String()
		if i == 0 { // create event does not produce a room section
			if len(res.Rooms.Join) != 0 {
				t.Fatalf("i=%v got %d joined rooms, want 0", i, len(res.Rooms.Join))
			}
		} else { // we should have that room somewhere
			if len(res.Rooms.Join) != 1 {
				t.Fatalf("i=%v got %d joined rooms, want 1", i, len(res.Rooms.Join))
			}
		}
	}

	// sync with no token "" and with the penultimate token and this should neatly return room events in the timeline block
	sinceTokens = append([]string{""}, sinceTokens[:len(sinceTokens)-1]...)

	t.Logf("waited for events to be consumed; syncing with %v", sinceTokens)
	for i, since := range sinceTokens {
		w := httptest.NewRecorder()
		routers.Client.ServeHTTP(w, test.NewRequest(t, "GET", "/_matrix/client/v3/sync", test.WithQueryParams(map[string]string{
			"access_token": alice.AccessToken,
			"timeout":      "0",
			"since":        since,
		})))
		if w.Code != 200 {
			t.Errorf("since=%s got HTTP %d want 200", since, w.Code)
		}
		var res types.Response
		if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
			t.Errorf("failed to decode response body: %s", err)
		}
		if len(res.Rooms.Join) != 1 {
			t.Fatalf("since=%s got %d joined rooms, want 1", since, len(res.Rooms.Join))
		}
		t.Logf("since=%s res state:%+v res timeline:%+v", since, res.Rooms.Join[room.ID].State.Events, res.Rooms.Join[room.ID].Timeline.Events)
		gotEventIDs := make([]string, len(res.Rooms.Join[room.ID].Timeline.Events))
		for j, ev := range res.Rooms.Join[room.ID].Timeline.Events {
			gotEventIDs[j] = ev.EventID
		}
		test.AssertEventIDsEqual(t, gotEventIDs, room.Events()[i:])
	}
}

// Test that if we hit /sync we get back presence: online, regardless of whether messages get delivered
// via NATS. Regression test for a flakey test "User sees their own presence in a sync"
func TestSyncAPIUpdatePresenceImmediately(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		testSyncAPIUpdatePresenceImmediately(t, dbType)
	})
}

func testSyncAPIUpdatePresenceImmediately(t *testing.T, dbType test.DBType) {
	user := test.NewUser(t)
	alice := userapi.Device{
		ID:          "ALICEID",
		UserID:      user.ID,
		AccessToken: "ALICE_BEARER_TOKEN",
		DisplayName: "Alice",
		AccountType: userapi.AccountTypeUser,
	}

	cfg, processCtx, close := testrig.CreateConfig(t, dbType)
	routers := httputil.NewRouters()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
	cfg.Global.Presence.EnableOutbound = true
	cfg.Global.Presence.EnableInbound = true
	defer close()
	natsInstance := jetstream.NATSInstance{}

	jsctx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
	defer jetstream.DeleteAllStreams(jsctx, &cfg.Global.JetStream)
	AddPublicRoutes(processCtx, routers, cfg, cm, &natsInstance, &syncUserAPI{accounts: []userapi.Device{alice}}, &syncRoomserverAPI{}, caches, caching.DisableMetrics)
	w := httptest.NewRecorder()
	routers.Client.ServeHTTP(w, test.NewRequest(t, "GET", "/_matrix/client/v3/sync", test.WithQueryParams(map[string]string{
		"access_token": alice.AccessToken,
		"timeout":      "0",
		"set_presence": "online",
	})))
	if w.Code != 200 {
		t.Fatalf("got HTTP %d want %d", w.Code, 200)
	}
	var res types.Response
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Errorf("failed to decode response body: %s", err)
	}
	if len(res.Presence.Events) != 1 {
		t.Fatalf("expected 1 presence events, got: %+v", res.Presence.Events)
	}
	if res.Presence.Events[0].Sender != alice.UserID {
		t.Errorf("sender: got %v want %v", res.Presence.Events[0].Sender, alice.UserID)
	}
	if res.Presence.Events[0].Type != "m.presence" {
		t.Errorf("type: got %v want %v", res.Presence.Events[0].Type, "m.presence")
	}
	if gjson.ParseBytes(res.Presence.Events[0].Content).Get("presence").Str != "online" {
		t.Errorf("content: not online,  got %v", res.Presence.Events[0].Content)
	}

}

// This is mainly what Sytest is doing in "test_history_visibility"
func TestMessageHistoryVisibility(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		testHistoryVisibility(t, dbType)
	})
}

func testHistoryVisibility(t *testing.T, dbType test.DBType) {
	type result struct {
		seeWithoutJoin bool
		seeBeforeJoin  bool
		seeAfterInvite bool
	}

	// create the users
	alice := test.NewUser(t)
	aliceDev := userapi.Device{
		ID:          "ALICEID",
		UserID:      alice.ID,
		AccessToken: "ALICE_BEARER_TOKEN",
		DisplayName: "ALICE",
	}

	bob := test.NewUser(t)

	bobDev := userapi.Device{
		ID:          "BOBID",
		UserID:      bob.ID,
		AccessToken: "BOD_BEARER_TOKEN",
		DisplayName: "BOB",
	}

	ctx := context.Background()
	// check guest and normal user accounts
	for _, accType := range []userapi.AccountType{userapi.AccountTypeGuest, userapi.AccountTypeUser} {
		testCases := []struct {
			historyVisibility gomatrixserverlib.HistoryVisibility
			wantResult        result
		}{
			{
				historyVisibility: gomatrixserverlib.HistoryVisibilityWorldReadable,
				wantResult: result{
					seeWithoutJoin: true,
					seeBeforeJoin:  true,
					seeAfterInvite: true,
				},
			},
			{
				historyVisibility: gomatrixserverlib.HistoryVisibilityShared,
				wantResult: result{
					seeWithoutJoin: false,
					seeBeforeJoin:  true,
					seeAfterInvite: true,
				},
			},
			{
				historyVisibility: gomatrixserverlib.HistoryVisibilityInvited,
				wantResult: result{
					seeWithoutJoin: false,
					seeBeforeJoin:  false,
					seeAfterInvite: true,
				},
			},
			{
				historyVisibility: gomatrixserverlib.HistoryVisibilityJoined,
				wantResult: result{
					seeWithoutJoin: false,
					seeBeforeJoin:  false,
					seeAfterInvite: false,
				},
			},
		}

		bobDev.AccountType = accType
		userType := "guest"
		if accType == userapi.AccountTypeUser {
			userType = "real user"
		}

		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cfg.ClientAPI.RateLimiting = config.RateLimiting{Enabled: false}
		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		defer close()
		natsInstance := jetstream.NATSInstance{}

		jsctx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
		defer jetstream.DeleteAllStreams(jsctx, &cfg.Global.JetStream)

		// Use the actual internal roomserver API
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		AddPublicRoutes(processCtx, routers, cfg, cm, &natsInstance, &syncUserAPI{accounts: []userapi.Device{aliceDev, bobDev}}, rsAPI, caches, caching.DisableMetrics)

		for _, tc := range testCases {
			testname := fmt.Sprintf("%s - %s", tc.historyVisibility, userType)
			t.Run(testname, func(t *testing.T) {
				// create a room with the given visibility
				room := test.NewRoom(t, alice, test.RoomHistoryVisibility(tc.historyVisibility))

				// send the events/messages to NATS to create the rooms
				beforeJoinBody := fmt.Sprintf("Before invite in a %s room", tc.historyVisibility)
				beforeJoinEv := room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": beforeJoinBody})
				eventsToSend := append(room.Events(), beforeJoinEv)
				if err := api.SendEvents(ctx, rsAPI, api.KindNew, eventsToSend, "test", "test", "test", nil, false); err != nil {
					t.Fatalf("failed to send events: %v", err)
				}
				syncUntil(t, routers, aliceDev.AccessToken, false,
					func(syncBody string) bool {
						path := fmt.Sprintf(`rooms.join.%s.timeline.events.#(content.body=="%s")`, room.ID, beforeJoinBody)
						return gjson.Get(syncBody, path).Exists()
					},
				)

				// There is only one event, we expect only to be able to see this, if the room is world_readable
				w := httptest.NewRecorder()
				routers.Client.ServeHTTP(w, test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/messages", room.ID), test.WithQueryParams(map[string]string{
					"access_token": bobDev.AccessToken,
					"dir":          "b",
					"filter":       `{"lazy_load_members":true}`, // check that lazy loading doesn't break history visibility
				})))
				if w.Code != 200 {
					t.Logf("%s", w.Body.String())
					t.Fatalf("got HTTP %d want %d", w.Code, 200)
				}
				// We only care about the returned events at this point
				var res struct {
					Chunk []synctypes.ClientEvent `json:"chunk"`
				}
				if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
					t.Errorf("failed to decode response body: %s", err)
				}

				verifyEventVisible(t, tc.wantResult.seeWithoutJoin, beforeJoinEv, res.Chunk)

				// Create invite, a message, join the room and create another message.
				inviteEv := room.CreateAndInsert(t, alice, "m.room.member", map[string]interface{}{"membership": "invite"}, test.WithStateKey(bob.ID))
				afterInviteEv := room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": fmt.Sprintf("After invite in a %s room", tc.historyVisibility)})
				joinEv := room.CreateAndInsert(t, bob, "m.room.member", map[string]interface{}{"membership": "join"}, test.WithStateKey(bob.ID))
				afterJoinBody := fmt.Sprintf("After join in a %s room", tc.historyVisibility)
				msgEv := room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": afterJoinBody})

				eventsToSend = append([]*rstypes.HeaderedEvent{}, inviteEv, afterInviteEv, joinEv, msgEv)

				if err := api.SendEvents(ctx, rsAPI, api.KindNew, eventsToSend, "test", "test", "test", nil, false); err != nil {
					t.Fatalf("failed to send events: %v", err)
				}
				syncUntil(t, routers, aliceDev.AccessToken, false,
					func(syncBody string) bool {
						path := fmt.Sprintf(`rooms.join.%s.timeline.events.#(content.body=="%s")`, room.ID, afterJoinBody)
						return gjson.Get(syncBody, path).Exists()
					},
				)

				// Verify the messages after/before invite are visible or not
				w = httptest.NewRecorder()
				routers.Client.ServeHTTP(w, test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/messages", room.ID), test.WithQueryParams(map[string]string{
					"access_token": bobDev.AccessToken,
					"dir":          "b",
				})))
				if w.Code != 200 {
					t.Logf("%s", w.Body.String())
					t.Fatalf("got HTTP %d want %d", w.Code, 200)
				}
				if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
					t.Errorf("failed to decode response body: %s", err)
				}
				// verify results
				verifyEventVisible(t, tc.wantResult.seeBeforeJoin, beforeJoinEv, res.Chunk)
				verifyEventVisible(t, tc.wantResult.seeAfterInvite, afterInviteEv, res.Chunk)
			})
		}
	}
}

func verifyEventVisible(t *testing.T, wantVisible bool, wantVisibleEvent *rstypes.HeaderedEvent, chunk []synctypes.ClientEvent) {
	t.Helper()
	if wantVisible {
		for _, ev := range chunk {
			if ev.EventID == wantVisibleEvent.EventID() {
				return
			}
		}
		t.Fatalf("expected to see event %s but didn't: %+v", wantVisibleEvent.EventID(), chunk)
	} else {
		for _, ev := range chunk {
			if ev.EventID == wantVisibleEvent.EventID() {
				t.Fatalf("expected not to see event %s: %+v", wantVisibleEvent.EventID(), string(ev.Content))
			}
		}
	}
}

func TestGetMembership(t *testing.T) {
	alice := test.NewUser(t)

	aliceDev := userapi.Device{
		ID:          "ALICEID",
		UserID:      alice.ID,
		AccessToken: "ALICE_BEARER_TOKEN",
		DisplayName: "Alice",
		AccountType: userapi.AccountTypeUser,
	}

	bob := test.NewUser(t)
	bobDev := userapi.Device{
		ID:          "BOBID",
		UserID:      bob.ID,
		AccessToken: "notjoinedtoanyrooms",
	}

	testCases := []struct {
		name             string
		roomID           string
		additionalEvents func(t *testing.T, room *test.Room)
		request          func(t *testing.T, room *test.Room) *http.Request
		wantOK           bool
		wantMemberCount  int
		useSleep         bool // :/
	}{
		{
			name: "/members - Alice joined",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/members", room.ID), test.WithQueryParams(map[string]string{
					"access_token": aliceDev.AccessToken,
				}))
			},
			wantOK:          true,
			wantMemberCount: 1,
		},
		{
			name: "/members - Bob never joined",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/members", room.ID), test.WithQueryParams(map[string]string{
					"access_token": bobDev.AccessToken,
				}))
			},
			wantOK: false,
		},
		{
			name: "/joined_members - Bob never joined",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/joined_members", room.ID), test.WithQueryParams(map[string]string{
					"access_token": bobDev.AccessToken,
				}))
			},
			wantOK: false,
		},
		{
			name: "/joined_members - Alice joined",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/joined_members", room.ID), test.WithQueryParams(map[string]string{
					"access_token": aliceDev.AccessToken,
				}))
			},
			wantOK: true,
		},
		{
			name: "Alice leaves before Bob joins, should not be able to see Bob",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/members", room.ID), test.WithQueryParams(map[string]string{
					"access_token": aliceDev.AccessToken,
				}))
			},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": "leave",
				}, test.WithStateKey(alice.ID))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
			},
			useSleep:        true,
			wantOK:          true,
			wantMemberCount: 1,
		},
		{
			name: "Alice leaves after Bob joins, should be able to see Bob",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/members", room.ID), test.WithQueryParams(map[string]string{
					"access_token": aliceDev.AccessToken,
				}))
			},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": "leave",
				}, test.WithStateKey(alice.ID))
			},
			useSleep:        true,
			wantOK:          true,
			wantMemberCount: 2,
		},
		{
			name: "/joined_members - Alice leaves, shouldn't be able to see members ",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/joined_members", room.ID), test.WithQueryParams(map[string]string{
					"access_token": aliceDev.AccessToken,
				}))
			},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": "leave",
				}, test.WithStateKey(alice.ID))
			},
			useSleep: true,
			wantOK:   false,
		},
		{
			name: "'at' specified, returns memberships before Bob joins",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/members", room.ID), test.WithQueryParams(map[string]string{
					"access_token": aliceDev.AccessToken,
					"at":           "t2_5",
				}))
			},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
			},
			useSleep:        true,
			wantOK:          true,
			wantMemberCount: 1,
		},
		{
			name: "'membership=leave' specified, returns no memberships",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/members", room.ID), test.WithQueryParams(map[string]string{
					"access_token": aliceDev.AccessToken,
					"membership":   "leave",
				}))
			},
			wantOK:          true,
			wantMemberCount: 0,
		},
		{
			name: "'not_membership=join' specified, returns no memberships",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/members", room.ID), test.WithQueryParams(map[string]string{
					"access_token":   aliceDev.AccessToken,
					"not_membership": "join",
				}))
			},
			wantOK:          true,
			wantMemberCount: 0,
		},
		{
			name: "'not_membership=leave' & 'membership=join' specified, returns correct memberships",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/members", room.ID), test.WithQueryParams(map[string]string{
					"access_token":   aliceDev.AccessToken,
					"not_membership": "leave",
					"membership":     "join",
				}))
			},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "leave",
				}, test.WithStateKey(bob.ID))
			},
			wantOK:          true,
			wantMemberCount: 1,
		},
		{
			name: "non-existent room ID",
			request: func(t *testing.T, room *test.Room) *http.Request {
				return test.NewRequest(t, "GET", fmt.Sprintf("/_matrix/client/v3/rooms/%s/members", "!notavalidroom:test"), test.WithQueryParams(map[string]string{
					"access_token": aliceDev.AccessToken,
				}))
			},
			wantOK: false,
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {

		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		defer close()
		natsInstance := jetstream.NATSInstance{}
		jsctx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
		defer jetstream.DeleteAllStreams(jsctx, &cfg.Global.JetStream)

		// Use an actual roomserver for this
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)

		AddPublicRoutes(processCtx, routers, cfg, cm, &natsInstance, &syncUserAPI{accounts: []userapi.Device{aliceDev, bobDev}}, rsAPI, caches, caching.DisableMetrics)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				room := test.NewRoom(t, alice)
				t.Cleanup(func() {
					t.Logf("running cleanup for %s", tc.name)
				})
				// inject additional events
				if tc.additionalEvents != nil {
					tc.additionalEvents(t, room)
				}
				if err := api.SendEvents(context.Background(), rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
					t.Fatalf("failed to send events: %v", err)
				}

				// wait for the events to come down sync
				if tc.useSleep {
					time.Sleep(time.Millisecond * 100)
				} else {
					syncUntil(t, routers, aliceDev.AccessToken, false, func(syncBody string) bool {
						// wait for the last sent eventID to come down sync
						path := fmt.Sprintf(`rooms.join.%s.timeline.events.#(event_id=="%s")`, room.ID, room.Events()[len(room.Events())-1].EventID())
						return gjson.Get(syncBody, path).Exists()
					})
				}

				w := httptest.NewRecorder()
				routers.Client.ServeHTTP(w, tc.request(t, room))
				if w.Code != 200 && tc.wantOK {
					t.Logf("%s", w.Body.String())
					t.Fatalf("got HTTP %d want %d", w.Code, 200)
				}
				t.Logf("[%s] Resp: %s", tc.name, w.Body.String())

				// check we got the expected events
				if tc.wantOK {
					memberCount := len(gjson.GetBytes(w.Body.Bytes(), "chunk").Array())
					if memberCount != tc.wantMemberCount {
						t.Fatalf("expected %d members, got %d", tc.wantMemberCount, memberCount)
					}
				}
			})
		}
	})
}

func TestSendToDevice(t *testing.T) {
	test.WithAllDatabases(t, testSendToDevice)
}

func testSendToDevice(t *testing.T, dbType test.DBType) {
	user := test.NewUser(t)
	alice := userapi.Device{
		ID:          "ALICEID",
		UserID:      user.ID,
		AccessToken: "ALICE_BEARER_TOKEN",
		DisplayName: "Alice",
		AccountType: userapi.AccountTypeUser,
	}

	cfg, processCtx, close := testrig.CreateConfig(t, dbType)
	routers := httputil.NewRouters()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
	defer close()
	natsInstance := jetstream.NATSInstance{}

	jsctx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
	defer jetstream.DeleteAllStreams(jsctx, &cfg.Global.JetStream)
	AddPublicRoutes(processCtx, routers, cfg, cm, &natsInstance, &syncUserAPI{accounts: []userapi.Device{alice}}, &syncRoomserverAPI{}, caches, caching.DisableMetrics)

	producer := producers.SyncAPIProducer{
		TopicSendToDeviceEvent: cfg.Global.JetStream.Prefixed(jetstream.OutputSendToDeviceEvent),
		JetStream:              jsctx,
	}

	msgCounter := 0

	testCases := []struct {
		name              string
		since             string
		want              []string
		sendMessagesCount int
	}{
		{
			name: "initial sync, no messages",
			want: []string{},
		},
		{
			name:              "initial sync, one new message",
			sendMessagesCount: 1,
			want: []string{
				"message 1",
			},
		},
		{
			name:              "initial sync, two new messages", // we didn't advance the since token, so we'll receive two messages
			sendMessagesCount: 1,
			want: []string{
				"message 1",
				"message 2",
			},
		},
		{
			name:  "incremental sync, one message", // this deletes message 1, as we advanced the since token
			since: types.StreamingToken{SendToDevicePosition: 1}.String(),
			want: []string{
				"message 2",
			},
		},
		{
			name:  "failed incremental sync, one message", // didn't advance since, so still the same message
			since: types.StreamingToken{SendToDevicePosition: 1}.String(),
			want: []string{
				"message 2",
			},
		},
		{
			name:  "incremental sync, no message",                         // this should delete message 2
			since: types.StreamingToken{SendToDevicePosition: 2}.String(), // next_batch from previous sync
			want:  []string{},
		},
		{
			name:              "incremental sync, three new messages",
			since:             types.StreamingToken{SendToDevicePosition: 2}.String(),
			sendMessagesCount: 3,
			want: []string{
				"message 3", // message 2 was deleted in the previous test
				"message 4",
				"message 5",
			},
		},
		{
			name: "initial sync, three messages", // we expect three messages, as we didn't go beyond "2"
			want: []string{
				"message 3",
				"message 4",
				"message 5",
			},
		},
		{
			name:  "incremental sync, no messages", // advance the sync token, no new messages
			since: types.StreamingToken{SendToDevicePosition: 5}.String(),
			want:  []string{},
		},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		// Send to-device messages of type "m.dendrite.test" with content `{"dummy":"message $counter"}`
		for i := 0; i < tc.sendMessagesCount; i++ {
			msgCounter++
			msg := json.RawMessage(fmt.Sprintf(`{"dummy":"message %d"}`, msgCounter))
			if err := producer.SendToDevice(ctx, user.ID, user.ID, alice.ID, "m.dendrite.test", msg); err != nil {
				t.Fatalf("unable to send to device message: %v", err)
			}
		}

		syncUntil(t, routers, alice.AccessToken,
			len(tc.want) == 0,
			func(body string) bool {
				return gjson.Get(body, fmt.Sprintf(`to_device.events.#(content.dummy=="message %d")`, msgCounter)).Exists()
			},
		)

		// Execute a /sync request, recording the response
		w := httptest.NewRecorder()
		routers.Client.ServeHTTP(w, test.NewRequest(t, "GET", "/_matrix/client/v3/sync", test.WithQueryParams(map[string]string{
			"access_token": alice.AccessToken,
			"since":        tc.since,
		})))

		// Extract the to_device.events, # gets all values of an array, in this case a string slice with "message $counter" entries
		events := gjson.Get(w.Body.String(), "to_device.events.#.content.dummy").Array()
		got := make([]string, len(events))
		for i := range events {
			got[i] = events[i].String()
		}

		// Ensure the messages we received are as we expect them to be
		if !reflect.DeepEqual(got, tc.want) {
			t.Logf("[%s|since=%s]: Sync: %s", tc.name, tc.since, w.Body.String())
			t.Fatalf("[%s|since=%s]: got: %+v, want: %+v", tc.name, tc.since, got, tc.want)
		}
	}
}

func TestContext(t *testing.T) {
	test.WithAllDatabases(t, testContext)
}

func testContext(t *testing.T, dbType test.DBType) {

	tests := []struct {
		name             string
		roomID           string
		eventID          string
		params           map[string]string
		wantError        bool
		wantStateLength  int
		wantBeforeLength int
		wantAfterLength  int
	}{
		{
			name: "invalid filter",
			params: map[string]string{
				"filter": "{",
			},
			wantError: true,
		},
		{
			name: "invalid limit",
			params: map[string]string{
				"limit": "abc",
			},
			wantError: true,
		},
		{
			name: "high limit",
			params: map[string]string{
				"limit": "100000",
			},
		},
		{
			name: "fine limit",
			params: map[string]string{
				"limit": "10",
			},
		},
		{
			name:            "last event without lazy loading",
			wantStateLength: 5,
		},
		{
			name: "last event with lazy loading",
			params: map[string]string{
				"filter": `{"lazy_load_members":true}`,
			},
			wantStateLength: 1,
		},
		{
			name:      "invalid room",
			roomID:    "!doesnotexist",
			wantError: true,
		},
		{
			name:      "invalid eventID",
			eventID:   "$doesnotexist",
			wantError: true,
		},
		{
			name: "state is limited",
			params: map[string]string{
				"limit": "1",
			},
			wantStateLength: 1,
		},
		{
			name:             "events are not limited",
			wantBeforeLength: 7,
		},
		{
			name: "all events are limited",
			params: map[string]string{
				"limit": "1",
			},
			wantStateLength:  1,
			wantBeforeLength: 1,
			wantAfterLength:  1,
		},
	}

	user := test.NewUser(t)
	alice := userapi.Device{
		ID:          "ALICEID",
		UserID:      user.ID,
		AccessToken: "ALICE_BEARER_TOKEN",
		DisplayName: "Alice",
		AccountType: userapi.AccountTypeUser,
	}

	cfg, processCtx, close := testrig.CreateConfig(t, dbType)
	routers := httputil.NewRouters()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
	defer close()

	// Use an actual roomserver for this
	natsInstance := jetstream.NATSInstance{}
	rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
	rsAPI.SetFederationAPI(nil, nil)

	AddPublicRoutes(processCtx, routers, cfg, cm, &natsInstance, &syncUserAPI{accounts: []userapi.Device{alice}}, rsAPI, caches, caching.DisableMetrics)

	room := test.NewRoom(t, user)

	room.CreateAndInsert(t, user, "m.room.message", map[string]interface{}{"body": "hello world 1!"})
	room.CreateAndInsert(t, user, "m.room.message", map[string]interface{}{"body": "hello world 2!"})
	thirdMsg := room.CreateAndInsert(t, user, "m.room.message", map[string]interface{}{"body": "hello world3!"})
	room.CreateAndInsert(t, user, "m.room.message", map[string]interface{}{"body": "hello world4!"})

	if err := api.SendEvents(context.Background(), rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
		t.Fatalf("failed to send events: %v", err)
	}

	jsctx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
	defer jetstream.DeleteAllStreams(jsctx, &cfg.Global.JetStream)

	syncUntil(t, routers, alice.AccessToken, false, func(syncBody string) bool {
		// wait for the last sent eventID to come down sync
		path := fmt.Sprintf(`rooms.join.%s.timeline.events.#(event_id=="%s")`, room.ID, thirdMsg.EventID())
		return gjson.Get(syncBody, path).Exists()
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]string{
				"access_token": alice.AccessToken,
			}
			w := httptest.NewRecorder()
			// test overrides
			roomID := room.ID
			if tc.roomID != "" {
				roomID = tc.roomID
			}
			eventID := thirdMsg.EventID()
			if tc.eventID != "" {
				eventID = tc.eventID
			}
			requestPath := fmt.Sprintf("/_matrix/client/v3/rooms/%s/context/%s", roomID, eventID)
			if tc.params != nil {
				for k, v := range tc.params {
					params[k] = v
				}
			}
			routers.Client.ServeHTTP(w, test.NewRequest(t, "GET", requestPath, test.WithQueryParams(params)))

			if tc.wantError && w.Code == 200 {
				t.Fatalf("Expected an error, but got none")
			}
			t.Log(w.Body.String())
			resp := routing.ContextRespsonse{}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if tc.wantStateLength > 0 && tc.wantStateLength != len(resp.State) {
				t.Fatalf("expected %d state events, got %d", tc.wantStateLength, len(resp.State))
			}
			if tc.wantBeforeLength > 0 && tc.wantBeforeLength != len(resp.EventsBefore) {
				t.Fatalf("expected %d before events, got %d", tc.wantBeforeLength, len(resp.EventsBefore))
			}
			if tc.wantAfterLength > 0 && tc.wantAfterLength != len(resp.EventsAfter) {
				t.Fatalf("expected %d after events, got %d", tc.wantAfterLength, len(resp.EventsAfter))
			}

			if !tc.wantError && resp.Event.EventID != eventID {
				t.Fatalf("unexpected eventID %s, expected %s", resp.Event.EventID, eventID)
			}
		})
	}
}

func TestUpdateRelations(t *testing.T) {
	testCases := []struct {
		name         string
		eventContent map[string]interface{}
		eventType    string
	}{
		{
			name: "empty event content should not error",
		},
		{
			name: "unable to unmarshal event should not error",
			eventContent: map[string]interface{}{
				"m.relates_to": map[string]interface{}{
					"event_id": map[string]interface{}{}, // this should be a string and not struct
				},
			},
		},
		{
			name: "empty event ID is ignored",
			eventContent: map[string]interface{}{
				"m.relates_to": map[string]interface{}{
					"event_id": "",
				},
			},
		},
		{
			name: "empty rel_type is ignored",
			eventContent: map[string]interface{}{
				"m.relates_to": map[string]interface{}{
					"event_id": "$randomEventID",
					"rel_type": "",
				},
			},
		},
		{
			name:      "redactions are ignored",
			eventType: spec.MRoomRedaction,
			eventContent: map[string]interface{}{
				"m.relates_to": map[string]interface{}{
					"event_id": "$randomEventID",
					"rel_type": "m.replace",
				},
			},
		},
		{
			name: "valid event is correctly written",
			eventContent: map[string]interface{}{
				"m.relates_to": map[string]interface{}{
					"event_id": "$randomEventID",
					"rel_type": "m.replace",
				},
			},
		},
	}

	ctx := context.Background()

	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		t.Cleanup(close)
		db, err := storage.NewSyncServerDatasource(processCtx.Context(), cm, &cfg.SyncAPI.Database)
		if err != nil {
			t.Fatal(err)
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				evType := "m.room.message"
				if tc.eventType != "" {
					evType = tc.eventType
				}
				ev := room.CreateEvent(t, alice, evType, tc.eventContent)
				err = db.UpdateRelations(ctx, ev)
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	})
}

func syncUntil(t *testing.T,
	routers httputil.Routers, accessToken string,
	skip bool,
	checkFunc func(syncBody string) bool,
) {
	t.Helper()
	if checkFunc == nil {
		t.Fatalf("No checkFunc defined")
	}
	if skip {
		return
	}
	// loop on /sync until we receive the last send message or timeout after 5 seconds, since we don't know if the message made it
	// to the syncAPI when hitting /sync
	done := make(chan bool)
	defer close(done)
	go func() {
		for {
			w := httptest.NewRecorder()
			routers.Client.ServeHTTP(w, test.NewRequest(t, "GET", "/_matrix/client/v3/sync", test.WithQueryParams(map[string]string{
				"access_token": accessToken,
				"timeout":      "1000",
			})))
			if checkFunc(w.Body.String()) {
				done <- true
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second * 5):
		t.Fatalf("Timed out waiting for messages")
	}
}

func toNATSMsgs(t *testing.T, cfg *config.Dendrite, input ...*rstypes.HeaderedEvent) []*nats.Msg {
	result := make([]*nats.Msg, len(input))
	for i, ev := range input {
		var addsStateIDs []string
		if ev.StateKey() != nil {
			addsStateIDs = append(addsStateIDs, ev.EventID())
		}
		result[i] = testrig.NewOutputEventMsg(t, cfg, ev.RoomID(), api.OutputEvent{
			Type: rsapi.OutputTypeNewRoomEvent,
			NewRoomEvent: &rsapi.OutputNewRoomEvent{
				Event:             ev,
				AddsStateEventIDs: addsStateIDs,
				HistoryVisibility: ev.Visibility,
			},
		})
	}
	return result
}
