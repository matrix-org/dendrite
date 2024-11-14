package roomserver_test

import (
	"context"
	"crypto/ed25519"
	"reflect"
	"testing"
	"time"

	"github.com/element-hq/dendrite/federationapi/statistics"
	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/eventutil"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/internal/input"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"

	"github.com/element-hq/dendrite/roomserver/acls"
	"github.com/element-hq/dendrite/roomserver/state"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/userapi"

	userAPI "github.com/element-hq/dendrite/userapi/api"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/element-hq/dendrite/federationapi"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/syncapi"

	"github.com/element-hq/dendrite/roomserver"
	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/storage"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/test/testrig"
)

var testIsBlacklistedOrBackingOff = func(s spec.ServerName) (*statistics.ServerStatistics, error) {
	return &statistics.ServerStatistics{}, nil
}

type FakeQuerier struct {
	api.QuerySenderIDAPI
}

func (f *FakeQuerier) QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	return spec.NewUserID(string(senderID), true)
}

func TestUsers(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		natsInstance := jetstream.NATSInstance{}
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		// SetFederationAPI starts the room event input consumer
		rsAPI.SetFederationAPI(nil, nil)

		t.Run("shared users", func(t *testing.T) {
			testSharedUsers(t, rsAPI)
		})

		t.Run("kick users", func(t *testing.T) {
			usrAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil, caching.DisableMetrics, testIsBlacklistedOrBackingOff)
			rsAPI.SetUserAPI(usrAPI)
			testKickUsers(t, rsAPI, usrAPI)
		})
	})

}

func testSharedUsers(t *testing.T, rsAPI api.RoomserverInternalAPI) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	room := test.NewRoom(t, alice, test.RoomPreset(test.PresetTrustedPrivateChat))

	// Invite and join Bob
	room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
		"membership": "invite",
	}, test.WithStateKey(bob.ID))
	room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
		"membership": "join",
	}, test.WithStateKey(bob.ID))

	ctx := context.Background()

	// Create the room
	if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
		t.Errorf("failed to send events: %v", err)
	}

	// Query the shared users for Alice, there should only be Bob.
	// This is used by the SyncAPI keychange consumer.
	res := &api.QuerySharedUsersResponse{}
	if err := rsAPI.QuerySharedUsers(ctx, &api.QuerySharedUsersRequest{UserID: alice.ID}, res); err != nil {
		t.Errorf("unable to query known users: %v", err)
	}
	if _, ok := res.UserIDsToCount[bob.ID]; !ok {
		t.Errorf("expected to find %s in shared users, but didn't: %+v", bob.ID, res.UserIDsToCount)
	}
	// Also verify that we get the expected result when specifying OtherUserIDs.
	// This is used by the SyncAPI when getting device list changes.
	if err := rsAPI.QuerySharedUsers(ctx, &api.QuerySharedUsersRequest{UserID: alice.ID, OtherUserIDs: []string{bob.ID}}, res); err != nil {
		t.Errorf("unable to query known users: %v", err)
	}
	if _, ok := res.UserIDsToCount[bob.ID]; !ok {
		t.Errorf("expected to find %s in shared users, but didn't: %+v", bob.ID, res.UserIDsToCount)
	}
}

func testKickUsers(t *testing.T, rsAPI api.RoomserverInternalAPI, usrAPI userAPI.UserInternalAPI) {
	// Create users and room; Bob is going to be the guest and kicked on revocation of guest access
	alice := test.NewUser(t, test.WithAccountType(userAPI.AccountTypeUser))
	bob := test.NewUser(t, test.WithAccountType(userAPI.AccountTypeGuest))

	room := test.NewRoom(t, alice, test.RoomPreset(test.PresetPublicChat), test.GuestsCanJoin(true))

	// Join with the guest user
	room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
		"membership": "join",
	}, test.WithStateKey(bob.ID))

	ctx := context.Background()

	// Create the users in the userapi, so the RSAPI can query the account type later
	for _, u := range []*test.User{alice, bob} {
		localpart, serverName, _ := gomatrixserverlib.SplitID('@', u.ID)
		userRes := &userAPI.PerformAccountCreationResponse{}
		if err := usrAPI.PerformAccountCreation(ctx, &userAPI.PerformAccountCreationRequest{
			AccountType: u.AccountType,
			Localpart:   localpart,
			ServerName:  serverName,
			Password:    "someRandomPassword",
		}, userRes); err != nil {
			t.Errorf("failed to create account: %s", err)
		}
	}

	// Create the room in the database
	if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
		t.Errorf("failed to send events: %v", err)
	}

	// Get the membership events BEFORE revoking guest access
	membershipRes := &api.QueryMembershipsForRoomResponse{}
	if err := rsAPI.QueryMembershipsForRoom(ctx, &api.QueryMembershipsForRoomRequest{LocalOnly: true, JoinedOnly: true, RoomID: room.ID}, membershipRes); err != nil {
		t.Errorf("failed to query membership for room: %s", err)
	}

	// revoke guest access
	revokeEvent := room.CreateAndInsert(t, alice, spec.MRoomGuestAccess, map[string]string{"guest_access": "forbidden"}, test.WithStateKey(""))
	if err := api.SendEvents(ctx, rsAPI, api.KindNew, []*types.HeaderedEvent{revokeEvent}, "test", "test", "test", nil, false); err != nil {
		t.Errorf("failed to send events: %v", err)
	}

	// TODO: Even though we are sending the events sync, the "kickUsers" function is sending the events async, so we need
	//		 to loop and wait for the events to be processed by the roomserver.
	for i := 0; i <= 20; i++ {
		// Get the membership events AFTER revoking guest access
		membershipRes2 := &api.QueryMembershipsForRoomResponse{}
		if err := rsAPI.QueryMembershipsForRoom(ctx, &api.QueryMembershipsForRoomRequest{LocalOnly: true, JoinedOnly: true, RoomID: room.ID}, membershipRes2); err != nil {
			t.Errorf("failed to query membership for room: %s", err)
		}

		// The membership events should NOT match, as Bob (guest user) should now be kicked from the room
		if !reflect.DeepEqual(membershipRes, membershipRes2) {
			return
		}
		time.Sleep(time.Millisecond * 10)
	}

	t.Errorf("memberships didn't change in time")
}

func Test_QueryLeftUsers(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	room := test.NewRoom(t, alice, test.RoomPreset(test.PresetTrustedPrivateChat))

	// Invite and join Bob
	room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
		"membership": "invite",
	}, test.WithStateKey(bob.ID))
	room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
		"membership": "join",
	}, test.WithStateKey(bob.ID))

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()

		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		natsInstance := jetstream.NATSInstance{}
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		// SetFederationAPI starts the room event input consumer
		rsAPI.SetFederationAPI(nil, nil)
		// Create the room
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatalf("failed to send events: %v", err)
		}

		// Query the left users, there should only be "@idontexist:test",
		// as Alice and Bob are still joined.
		res := &api.QueryLeftUsersResponse{}
		leftUserID := "@idontexist:test"
		getLeftUsersList := []string{alice.ID, bob.ID, leftUserID}

		testCase := func(rsAPI api.RoomserverInternalAPI) {
			if err := rsAPI.QueryLeftUsers(ctx, &api.QueryLeftUsersRequest{StaleDeviceListUsers: getLeftUsersList}, res); err != nil {
				t.Fatalf("unable to query left users: %v", err)
			}
			wantCount := 1
			if count := len(res.LeftUsers); count > wantCount {
				t.Fatalf("unexpected left users count: want %d, got %d", wantCount, count)
			}
			if res.LeftUsers[0] != leftUserID {
				t.Fatalf("unexpected left users : want %s, got %s", leftUserID, res.LeftUsers[0])
			}
		}

		testCase(rsAPI)
	})
}

func TestPurgeRoom(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	room := test.NewRoom(t, alice, test.RoomPreset(test.PresetTrustedPrivateChat))

	roomID, err := spec.NewRoomID(room.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Invite Bob
	inviteEvent := room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
		"membership": "invite",
	}, test.WithStateKey(bob.ID))

	ctx := context.Background()

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		natsInstance := jetstream.NATSInstance{}
		defer close()
		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		db, err := storage.Open(processCtx.Context(), cm, &cfg.RoomServer.Database, caches)
		if err != nil {
			t.Fatal(err)
		}
		jsCtx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
		defer jetstream.DeleteAllStreams(jsCtx, &cfg.Global.JetStream)

		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)

		// this starts the JetStream consumers
		fsAPI := federationapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, nil, rsAPI, caches, nil, true)
		rsAPI.SetFederationAPI(fsAPI, nil)

		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil, caching.DisableMetrics, fsAPI.IsBlacklistedOrBackingOff)
		syncapi.AddPublicRoutes(processCtx, routers, cfg, cm, &natsInstance, userAPI, rsAPI, caches, caching.DisableMetrics)

		// Create the room
		if err = api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatalf("failed to send events: %v", err)
		}

		// some dummy entries to validate after purging
		if err = rsAPI.PerformPublish(ctx, &api.PerformPublishRequest{RoomID: room.ID, Visibility: spec.Public}); err != nil {
			t.Fatal(err)
		}

		isPublished, err := db.GetPublishedRoom(ctx, room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !isPublished {
			t.Fatalf("room should be published before purging")
		}
		if _, err = rsAPI.SetRoomAlias(ctx, spec.SenderID(alice.ID), *roomID, "myalias"); err != nil {
			t.Fatal(err)
		}
		// check the alias is actually there
		aliasesResp := &api.GetAliasesForRoomIDResponse{}
		if err = rsAPI.GetAliasesForRoomID(ctx, &api.GetAliasesForRoomIDRequest{RoomID: room.ID}, aliasesResp); err != nil {
			t.Fatal(err)
		}
		wantAliases := 1
		if gotAliases := len(aliasesResp.Aliases); gotAliases != wantAliases {
			t.Fatalf("expected %d aliases, got %d", wantAliases, gotAliases)
		}

		// validate the room exists before purging
		roomInfo, err := db.RoomInfo(ctx, room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if roomInfo == nil {
			t.Fatalf("room does not exist")
		}

		//
		roomInfo2, err := db.RoomInfoByNID(ctx, roomInfo.RoomNID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(roomInfo, roomInfo2) {
			t.Fatalf("expected roomInfos to be the same, but they aren't")
		}

		// remember the roomInfo before purging
		existingRoomInfo := roomInfo

		// validate there is an invite for bob
		nids, err := db.EventStateKeyNIDs(ctx, []string{bob.ID})
		if err != nil {
			t.Fatal(err)
		}
		bobNID, ok := nids[bob.ID]
		if !ok {
			t.Fatalf("%s does not exist", bob.ID)
		}

		_, inviteEventIDs, _, err := db.GetInvitesForUser(ctx, roomInfo.RoomNID, bobNID)
		if err != nil {
			t.Fatal(err)
		}
		wantInviteCount := 1
		if inviteCount := len(inviteEventIDs); inviteCount != wantInviteCount {
			t.Fatalf("expected there to be only %d invite events, got %d", wantInviteCount, inviteCount)
		}
		if inviteEventIDs[0] != inviteEvent.EventID() {
			t.Fatalf("expected invite event ID %s, got %s", inviteEvent.EventID(), inviteEventIDs[0])
		}

		// purge the room from the database
		if err = rsAPI.PerformAdminPurgeRoom(ctx, room.ID); err != nil {
			t.Fatal(err)
		}

		// wait for all consumers to process the purge event
		var sum = 1
		timeout := time.Second * 5
		deadline, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		for sum > 0 {
			if deadline.Err() != nil {
				t.Fatalf("test timed out after %s", timeout)
			}
			sum = 0
			consumerCh := jsCtx.Consumers(cfg.Global.JetStream.Prefixed(jetstream.OutputRoomEvent))
			for x := range consumerCh {
				sum += x.NumAckPending
			}
			time.Sleep(time.Millisecond)
		}

		roomInfo, err = db.RoomInfo(ctx, room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if roomInfo != nil {
			t.Fatalf("room should not exist after purging: %+v", roomInfo)
		}
		roomInfo2, err = db.RoomInfoByNID(ctx, existingRoomInfo.RoomNID)
		if err == nil {
			t.Fatalf("expected room to not exist, but it does: %#v", roomInfo2)
		}

		// validation below

		// There should be no invite left
		_, inviteEventIDs, _, err = db.GetInvitesForUser(ctx, existingRoomInfo.RoomNID, bobNID)
		if err != nil {
			t.Fatal(err)
		}

		if inviteCount := len(inviteEventIDs); inviteCount > 0 {
			t.Fatalf("expected there to be only %d invite events, got %d", wantInviteCount, inviteCount)
		}

		// aliases should be deleted
		aliases, err := db.GetAliasesForRoomID(ctx, room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if aliasCount := len(aliases); aliasCount > 0 {
			t.Fatalf("expected there to be only %d invite events, got %d", 0, aliasCount)
		}

		// published room should be deleted
		isPublished, err = db.GetPublishedRoom(ctx, room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if isPublished {
			t.Fatalf("room should not be published after purging")
		}
	})
}

type fledglingEvent struct {
	Type       string
	StateKey   *string
	SenderID   string
	RoomID     string
	Redacts    string
	Depth      int64
	PrevEvents []any
	AuthEvents []any
	Content    map[string]any
}

func mustCreateEvent(t *testing.T, ev fledglingEvent) (result *types.HeaderedEvent) {
	t.Helper()
	roomVer := gomatrixserverlib.RoomVersionV9
	seed := make([]byte, ed25519.SeedSize) // zero seed
	key := ed25519.NewKeyFromSeed(seed)
	eb := gomatrixserverlib.MustGetRoomVersion(roomVer).NewEventBuilderFromProtoEvent(&gomatrixserverlib.ProtoEvent{
		SenderID:   ev.SenderID,
		Type:       ev.Type,
		StateKey:   ev.StateKey,
		RoomID:     ev.RoomID,
		Redacts:    ev.Redacts,
		Depth:      ev.Depth,
		PrevEvents: ev.PrevEvents,
	})
	if ev.Content == nil {
		ev.Content = map[string]any{}
	}
	if ev.AuthEvents != nil {
		eb.AuthEvents = ev.AuthEvents
	}
	err := eb.SetContent(ev.Content)
	if err != nil {
		t.Fatalf("mustCreateEvent: failed to marshal event content %v", err)
	}

	signedEvent, err := eb.Build(time.Now(), "localhost", "ed25519:test", key)
	if err != nil {
		t.Fatalf("mustCreateEvent: failed to sign event: %s", err)
	}
	h := &types.HeaderedEvent{PDU: signedEvent}
	return h
}

func TestRedaction(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	charlie := test.NewUser(t, test.WithSigningServer("notlocalhost", "abc", test.PrivateKeyB))

	testCases := []struct {
		name             string
		additionalEvents func(t *testing.T, room *test.Room)
		wantRedacted     bool
	}{
		{
			name:         "can redact own message",
			wantRedacted: true,
			additionalEvents: func(t *testing.T, room *test.Room) {
				redactedEvent := room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": "hello world"})

				builderEv := mustCreateEvent(t, fledglingEvent{
					Type:       spec.MRoomRedaction,
					SenderID:   alice.ID,
					RoomID:     room.ID,
					Redacts:    redactedEvent.EventID(),
					Depth:      redactedEvent.Depth() + 1,
					PrevEvents: []interface{}{redactedEvent.EventID()},
				})
				room.InsertEvent(t, builderEv)
			},
		},
		{
			name:         "can redact others message, allowed by PL",
			wantRedacted: true,
			additionalEvents: func(t *testing.T, room *test.Room) {
				redactedEvent := room.CreateAndInsert(t, bob, "m.room.message", map[string]interface{}{"body": "hello world"})

				builderEv := mustCreateEvent(t, fledglingEvent{
					Type:       spec.MRoomRedaction,
					SenderID:   alice.ID,
					RoomID:     room.ID,
					Redacts:    redactedEvent.EventID(),
					Depth:      redactedEvent.Depth() + 1,
					PrevEvents: []interface{}{redactedEvent.EventID()},
				})
				room.InsertEvent(t, builderEv)
			},
		},
		{
			name:         "can redact others message, same server",
			wantRedacted: true,
			additionalEvents: func(t *testing.T, room *test.Room) {
				redactedEvent := room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": "hello world"})

				builderEv := mustCreateEvent(t, fledglingEvent{
					Type:       spec.MRoomRedaction,
					SenderID:   bob.ID,
					RoomID:     room.ID,
					Redacts:    redactedEvent.EventID(),
					Depth:      redactedEvent.Depth() + 1,
					PrevEvents: []interface{}{redactedEvent.EventID()},
				})
				room.InsertEvent(t, builderEv)
			},
		},
		{
			name: "can not redact others message, missing PL",
			additionalEvents: func(t *testing.T, room *test.Room) {
				redactedEvent := room.CreateAndInsert(t, bob, "m.room.message", map[string]interface{}{"body": "hello world"})

				builderEv := mustCreateEvent(t, fledglingEvent{
					Type:       spec.MRoomRedaction,
					SenderID:   charlie.ID,
					RoomID:     room.ID,
					Redacts:    redactedEvent.EventID(),
					Depth:      redactedEvent.Depth() + 1,
					PrevEvents: []interface{}{redactedEvent.EventID()},
				})
				room.InsertEvent(t, builderEv)
			},
		},
	}

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		db, err := storage.Open(processCtx.Context(), cm, &cfg.RoomServer.Database, caches)
		if err != nil {
			t.Fatal(err)
		}

		natsInstance := &jetstream.NATSInstance{}
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				authEvents := []types.EventNID{}
				var roomInfo *types.RoomInfo
				var err error

				room := test.NewRoom(t, alice, test.RoomPreset(test.PresetPublicChat))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, charlie, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(charlie.ID))

				if tc.additionalEvents != nil {
					tc.additionalEvents(t, room)
				}

				for _, ev := range room.Events() {
					roomInfo, err = db.GetOrCreateRoomInfo(ctx, ev.PDU)
					assert.NoError(t, err)
					assert.NotNil(t, roomInfo)
					evTypeNID, err := db.GetOrCreateEventTypeNID(ctx, ev.Type())
					assert.NoError(t, err)

					stateKeyNID, err := db.GetOrCreateEventStateKeyNID(ctx, ev.StateKey())
					assert.NoError(t, err)

					eventNID, stateAtEvent, err := db.StoreEvent(ctx, ev.PDU, roomInfo, evTypeNID, stateKeyNID, authEvents, false)
					assert.NoError(t, err)
					if ev.StateKey() != nil {
						authEvents = append(authEvents, eventNID)
					}

					// Calculate the snapshotNID etc.
					plResolver := state.NewStateResolution(db, roomInfo, rsAPI)
					stateAtEvent.BeforeStateSnapshotNID, err = plResolver.CalculateAndStoreStateBeforeEvent(ctx, ev.PDU, false)
					assert.NoError(t, err)

					// Update the room
					updater, err := db.GetRoomUpdater(ctx, roomInfo)
					assert.NoError(t, err)
					err = updater.SetState(ctx, eventNID, stateAtEvent.BeforeStateSnapshotNID)
					assert.NoError(t, err)
					err = updater.Commit()
					assert.NoError(t, err)

					_, redactedEvent, err := db.MaybeRedactEvent(ctx, roomInfo, eventNID, ev.PDU, &plResolver, &FakeQuerier{})
					assert.NoError(t, err)
					if redactedEvent != nil {
						assert.Equal(t, ev.Redacts(), redactedEvent.EventID())
					}
					if ev.Type() == spec.MRoomRedaction {
						nids, err := db.EventNIDs(ctx, []string{ev.Redacts()})
						assert.NoError(t, err)
						evs, err := db.Events(ctx, roomInfo.RoomVersion, []types.EventNID{nids[ev.Redacts()].EventNID})
						assert.NoError(t, err)
						assert.Equal(t, 1, len(evs))
						assert.Equal(t, tc.wantRedacted, evs[0].Redacted())
					}
				}
			})
		}
	})
}

func TestQueryRestrictedJoinAllowed(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)

	// a room we don't create in the database
	allowedByRoomNotExists := test.NewRoom(t, alice)

	// a room we create in the database, used for authorisation
	allowedByRoomExists := test.NewRoom(t, alice)
	allowedByRoomExists.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
		"membership": spec.Join,
	}, test.WithStateKey(bob.ID))

	testCases := []struct {
		name            string
		prepareRoomFunc func(t *testing.T) *test.Room
		wantResponse    string
		wantError       bool
	}{
		{
			name: "public room unrestricted",
			prepareRoomFunc: func(t *testing.T) *test.Room {
				return test.NewRoom(t, alice)
			},
			wantResponse: "",
		},
		{
			name: "room version without restrictions",
			prepareRoomFunc: func(t *testing.T) *test.Room {
				return test.NewRoom(t, alice, test.RoomVersion(gomatrixserverlib.RoomVersionV7))
			},
		},
		{
			name: "restricted only", // bob is not allowed to join
			prepareRoomFunc: func(t *testing.T) *test.Room {
				r := test.NewRoom(t, alice, test.RoomVersion(gomatrixserverlib.RoomVersionV8))
				r.CreateAndInsert(t, alice, spec.MRoomJoinRules, map[string]interface{}{
					"join_rule": spec.Restricted,
				}, test.WithStateKey(""))
				return r
			},
			wantError: true,
		},
		{
			name: "knock_restricted",
			prepareRoomFunc: func(t *testing.T) *test.Room {
				r := test.NewRoom(t, alice, test.RoomVersion(gomatrixserverlib.RoomVersionV8))
				r.CreateAndInsert(t, alice, spec.MRoomJoinRules, map[string]interface{}{
					"join_rule": spec.KnockRestricted,
				}, test.WithStateKey(""))
				return r
			},
			wantError: true,
		},
		{
			name: "restricted with pending invite", // bob should be allowed to join
			prepareRoomFunc: func(t *testing.T) *test.Room {
				r := test.NewRoom(t, alice, test.RoomVersion(gomatrixserverlib.RoomVersionV8))
				r.CreateAndInsert(t, alice, spec.MRoomJoinRules, map[string]interface{}{
					"join_rule": spec.Restricted,
				}, test.WithStateKey(""))
				r.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": spec.Invite,
				}, test.WithStateKey(bob.ID))
				return r
			},
			wantResponse: "",
		},
		{
			name: "restricted with allowed room_id, but missing room", // bob should not be allowed to join, as we don't know about the room
			prepareRoomFunc: func(t *testing.T) *test.Room {
				r := test.NewRoom(t, alice, test.RoomVersion(gomatrixserverlib.RoomVersionV10))
				r.CreateAndInsert(t, alice, spec.MRoomJoinRules, map[string]interface{}{
					"join_rule": spec.KnockRestricted,
					"allow": []map[string]interface{}{
						{
							"room_id": allowedByRoomNotExists.ID,
							"type":    spec.MRoomMembership,
						},
					},
				}, test.WithStateKey(""))
				r.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership":                       spec.Join,
					"join_authorised_via_users_server": alice.ID,
				}, test.WithStateKey(bob.ID))
				return r
			},
			wantError: true,
		},
		{
			name: "restricted with allowed room_id", // bob should be allowed to join, as we know about the room
			prepareRoomFunc: func(t *testing.T) *test.Room {
				r := test.NewRoom(t, alice, test.RoomVersion(gomatrixserverlib.RoomVersionV10))
				r.CreateAndInsert(t, alice, spec.MRoomJoinRules, map[string]interface{}{
					"join_rule": spec.KnockRestricted,
					"allow": []map[string]interface{}{
						{
							"room_id": allowedByRoomExists.ID,
							"type":    spec.MRoomMembership,
						},
					},
				}, test.WithStateKey(""))
				r.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership":                       spec.Join,
					"join_authorised_via_users_server": alice.ID,
				}, test.WithStateKey(bob.ID))
				return r
			},
			wantResponse: alice.ID,
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		natsInstance := jetstream.NATSInstance{}
		defer close()

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)

		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.prepareRoomFunc == nil {
					t.Fatal("missing prepareRoomFunc")
				}
				testRoom := tc.prepareRoomFunc(t)
				// Create the room
				if err := api.SendEvents(processCtx.Context(), rsAPI, api.KindNew, testRoom.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}

				if err := api.SendEvents(processCtx.Context(), rsAPI, api.KindNew, allowedByRoomExists.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}

				roomID, _ := spec.NewRoomID(testRoom.ID)
				userID, _ := spec.NewUserID(bob.ID, true)
				got, err := rsAPI.QueryRestrictedJoinAllowed(processCtx.Context(), *roomID, spec.SenderID(userID.String()))
				if tc.wantError && err == nil {
					t.Fatal("expected error, got none")
				}
				if !tc.wantError && err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(tc.wantResponse, got) {
					t.Fatalf("unexpected response, want %#v - got %#v", tc.wantResponse, got)
				}
			})
		}
	})
}

func TestUpgrade(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	charlie := test.NewUser(t)
	ctx := context.Background()

	spaceChild := test.NewRoom(t, alice)
	validateTuples := []gomatrixserverlib.StateKeyTuple{
		{EventType: spec.MRoomCreate},
		{EventType: spec.MRoomPowerLevels},
		{EventType: spec.MRoomJoinRules},
		{EventType: spec.MRoomName},
		{EventType: spec.MRoomCanonicalAlias},
		{EventType: "m.room.tombstone"},
		{EventType: "m.custom.event"},
		{EventType: "m.space.child", StateKey: spaceChild.ID},
		{EventType: "m.custom.event", StateKey: alice.ID},
		{EventType: spec.MRoomMember, StateKey: charlie.ID}, // ban should be transferred
	}

	validate := func(t *testing.T, oldRoomID, newRoomID string, rsAPI api.RoomserverInternalAPI) {

		oldRoomState := &api.QueryCurrentStateResponse{}
		if err := rsAPI.QueryCurrentState(ctx, &api.QueryCurrentStateRequest{
			RoomID:      oldRoomID,
			StateTuples: validateTuples,
		}, oldRoomState); err != nil {
			t.Fatal(err)
		}

		newRoomState := &api.QueryCurrentStateResponse{}
		if err := rsAPI.QueryCurrentState(ctx, &api.QueryCurrentStateRequest{
			RoomID:      newRoomID,
			StateTuples: validateTuples,
		}, newRoomState); err != nil {
			t.Fatal(err)
		}

		// the old room should have a tombstone event
		ev := oldRoomState.StateEvents[gomatrixserverlib.StateKeyTuple{EventType: "m.room.tombstone"}]
		replacementRoom := gjson.GetBytes(ev.Content(), "replacement_room").Str
		if replacementRoom != newRoomID {
			t.Fatalf("tombstone event has replacement_room '%s', expected '%s'", replacementRoom, newRoomID)
		}

		// the new room should have a predecessor equal to the old room
		ev = newRoomState.StateEvents[gomatrixserverlib.StateKeyTuple{EventType: spec.MRoomCreate}]
		predecessor := gjson.GetBytes(ev.Content(), "predecessor.room_id").Str
		if predecessor != oldRoomID {
			t.Fatalf("got predecessor room '%s', expected '%s'", predecessor, oldRoomID)
		}

		for _, tuple := range validateTuples {
			// Skip create and powerlevel event (new room has e.g. predecessor event, old room has restricted powerlevels)
			switch tuple.EventType {
			case spec.MRoomCreate, spec.MRoomPowerLevels, spec.MRoomCanonicalAlias:
				continue
			}
			oldEv, ok := oldRoomState.StateEvents[tuple]
			if !ok {
				t.Logf("skipping tuple %#v as it doesn't exist in the old room", tuple)
				continue
			}
			newEv, ok := newRoomState.StateEvents[tuple]
			if !ok {
				t.Logf("skipping tuple %#v as it doesn't exist in the new room", tuple)
				continue
			}

			if !reflect.DeepEqual(oldEv.Content(), newEv.Content()) {
				t.Logf("OldEvent QueryCurrentState: %s", string(oldEv.Content()))
				t.Logf("NewEvent QueryCurrentState: %s", string(newEv.Content()))
				t.Errorf("event content mismatch")
			}
		}
	}

	testCases := []struct {
		name         string
		upgradeUser  string
		roomFunc     func(rsAPI api.RoomserverInternalAPI) string
		validateFunc func(t *testing.T, oldRoomID, newRoomID string, rsAPI api.RoomserverInternalAPI)
		wantNewRoom  bool
	}{
		{
			name:        "invalid roomID",
			upgradeUser: alice.ID,
			roomFunc: func(rsAPI api.RoomserverInternalAPI) string {
				return "!doesnotexist:test"
			},
		},
		{
			name:        "powerlevel too low",
			upgradeUser: bob.ID,
			roomFunc: func(rsAPI api.RoomserverInternalAPI) string {
				room := test.NewRoom(t, alice)
				if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}
				return room.ID
			},
		},
		{
			name:        "successful upgrade on new room",
			upgradeUser: alice.ID,
			roomFunc: func(rsAPI api.RoomserverInternalAPI) string {
				room := test.NewRoom(t, alice)
				if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}
				return room.ID
			},
			wantNewRoom:  true,
			validateFunc: validate,
		},
		{
			name:        "successful upgrade on new room with other state events",
			upgradeUser: alice.ID,
			roomFunc: func(rsAPI api.RoomserverInternalAPI) string {
				r := test.NewRoom(t, alice)
				r.CreateAndInsert(t, alice, spec.MRoomName, map[string]interface{}{
					"name": "my new name",
				}, test.WithStateKey(""))
				r.CreateAndInsert(t, alice, spec.MRoomCanonicalAlias, eventutil.CanonicalAliasContent{
					Alias: "#myalias:test",
				}, test.WithStateKey(""))

				// this will be transferred
				r.CreateAndInsert(t, alice, "m.custom.event", map[string]interface{}{
					"random": "i should exist",
				}, test.WithStateKey(""))

				// the following will be ignored
				r.CreateAndInsert(t, alice, "m.custom.event", map[string]interface{}{
					"random": "i will be ignored",
				}, test.WithStateKey(alice.ID))

				if err := api.SendEvents(ctx, rsAPI, api.KindNew, r.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}
				return r.ID
			},
			wantNewRoom:  true,
			validateFunc: validate,
		},
		{
			name:        "with published room",
			upgradeUser: alice.ID,
			roomFunc: func(rsAPI api.RoomserverInternalAPI) string {
				r := test.NewRoom(t, alice)
				if err := api.SendEvents(ctx, rsAPI, api.KindNew, r.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}

				if err := rsAPI.PerformPublish(ctx, &api.PerformPublishRequest{
					RoomID:     r.ID,
					Visibility: spec.Public,
				}); err != nil {
					t.Fatal(err)
				}

				return r.ID
			},
			wantNewRoom: true,
			validateFunc: func(t *testing.T, oldRoomID, newRoomID string, rsAPI api.RoomserverInternalAPI) {
				validate(t, oldRoomID, newRoomID, rsAPI)
				// check that the new room is published
				res := &api.QueryPublishedRoomsResponse{}
				if err := rsAPI.QueryPublishedRooms(ctx, &api.QueryPublishedRoomsRequest{RoomID: newRoomID}, res); err != nil {
					t.Fatal(err)
				}
				if len(res.RoomIDs) == 0 {
					t.Fatalf("expected room to be published, but wasn't: %#v", res.RoomIDs)
				}
			},
		},
		{
			name:        "with alias",
			upgradeUser: alice.ID,
			roomFunc: func(rsAPI api.RoomserverInternalAPI) string {
				r := test.NewRoom(t, alice)
				roomID, err := spec.NewRoomID(r.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := api.SendEvents(ctx, rsAPI, api.KindNew, r.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}

				if _, err := rsAPI.SetRoomAlias(ctx, spec.SenderID(alice.ID),
					*roomID,
					"#myroomalias:test"); err != nil {
					t.Fatal(err)
				}

				return r.ID
			},
			wantNewRoom: true,
			validateFunc: func(t *testing.T, oldRoomID, newRoomID string, rsAPI api.RoomserverInternalAPI) {
				validate(t, oldRoomID, newRoomID, rsAPI)
				// check that the old room has no aliases
				res := &api.GetAliasesForRoomIDResponse{}
				if err := rsAPI.GetAliasesForRoomID(ctx, &api.GetAliasesForRoomIDRequest{RoomID: oldRoomID}, res); err != nil {
					t.Fatal(err)
				}
				if len(res.Aliases) != 0 {
					t.Fatalf("expected old room aliases to be empty, but wasn't: %#v", res.Aliases)
				}

				// check that the new room has aliases
				if err := rsAPI.GetAliasesForRoomID(ctx, &api.GetAliasesForRoomIDRequest{RoomID: newRoomID}, res); err != nil {
					t.Fatal(err)
				}
				if len(res.Aliases) == 0 {
					t.Fatalf("expected room aliases to be transferred, but wasn't: %#v", res.Aliases)
				}
			},
		},
		{
			name:        "bans are transferred",
			upgradeUser: alice.ID,
			roomFunc: func(rsAPI api.RoomserverInternalAPI) string {
				r := test.NewRoom(t, alice)
				r.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": spec.Ban,
				}, test.WithStateKey(charlie.ID))
				if err := api.SendEvents(ctx, rsAPI, api.KindNew, r.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}
				return r.ID
			},
			wantNewRoom:  true,
			validateFunc: validate,
		},
		{
			name:        "space childs are transferred",
			upgradeUser: alice.ID,
			roomFunc: func(rsAPI api.RoomserverInternalAPI) string {
				r := test.NewRoom(t, alice)

				r.CreateAndInsert(t, alice, "m.space.child", map[string]interface{}{}, test.WithStateKey(spaceChild.ID))
				if err := api.SendEvents(ctx, rsAPI, api.KindNew, r.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}
				return r.ID
			},
			wantNewRoom:  true,
			validateFunc: validate,
		},
		{
			name:        "custom state is not taken to the new room", // https://github.com/element-hq/dendrite/issues/2912
			upgradeUser: charlie.ID,
			roomFunc: func(rsAPI api.RoomserverInternalAPI) string {
				r := test.NewRoom(t, alice, test.RoomVersion(gomatrixserverlib.RoomVersionV6))
				// Bob and Charlie join
				r.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{"membership": spec.Join}, test.WithStateKey(bob.ID))
				r.CreateAndInsert(t, charlie, spec.MRoomMember, map[string]interface{}{"membership": spec.Join}, test.WithStateKey(charlie.ID))

				// make Charlie an admin so the room can be upgraded
				r.CreateAndInsert(t, alice, spec.MRoomPowerLevels, gomatrixserverlib.PowerLevelContent{
					Users: map[string]int64{
						charlie.ID: 100,
					},
				}, test.WithStateKey(""))

				// Alice creates a custom event
				r.CreateAndInsert(t, alice, "m.custom.event", map[string]interface{}{
					"random": "data",
				}, test.WithStateKey(alice.ID))
				r.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{"membership": spec.Leave}, test.WithStateKey(alice.ID))

				if err := api.SendEvents(ctx, rsAPI, api.KindNew, r.Events(), "test", "test", "test", nil, false); err != nil {
					t.Errorf("failed to send events: %v", err)
				}
				return r.ID
			},
			wantNewRoom:  true,
			validateFunc: validate,
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		natsInstance := jetstream.NATSInstance{}
		defer close()

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)

		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil, caching.DisableMetrics, testIsBlacklistedOrBackingOff)
		rsAPI.SetUserAPI(userAPI)

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.roomFunc == nil {
					t.Fatalf("missing roomFunc")
				}
				if tc.upgradeUser == "" {
					tc.upgradeUser = alice.ID
				}
				roomID := tc.roomFunc(rsAPI)

				userID, err := spec.NewUserID(tc.upgradeUser, true)
				if err != nil {
					t.Fatalf("upgrade userID is invalid")
				}
				newRoomID, err := rsAPI.PerformRoomUpgrade(processCtx.Context(), roomID, *userID, rsAPI.DefaultRoomVersion())
				if err != nil && tc.wantNewRoom {
					t.Fatal(err)
				}

				if tc.wantNewRoom && newRoomID == "" {
					t.Fatalf("expected a new room, but the upgrade failed")
				}
				if !tc.wantNewRoom && newRoomID != "" {
					t.Fatalf("expected no new room, but the upgrade succeeded")
				}
				if tc.validateFunc != nil {
					tc.validateFunc(t, roomID, newRoomID, rsAPI)
				}
			})
		}
	})
}

func TestStateReset(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	charlie := test.NewUser(t)
	ctx := context.Background()

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		// Prepare APIs
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		natsInstance := jetstream.NATSInstance{}
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)

		// create a new room
		room := test.NewRoom(t, alice, test.RoomPreset(test.PresetPublicChat))

		// join with Bob and Charlie
		bobJoinEv := room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]any{"membership": "join"}, test.WithStateKey(bob.ID))
		charlieJoinEv := room.CreateAndInsert(t, charlie, spec.MRoomMember, map[string]any{"membership": "join"}, test.WithStateKey(charlie.ID))

		// Send and create the room
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Errorf("failed to send events: %v", err)
		}

		// send a message
		bobMsg := room.CreateAndInsert(t, bob, "m.room.message", map[string]any{"body": "hello world"})
		charlieMsg := room.CreateAndInsert(t, charlie, "m.room.message", map[string]any{"body": "hello world"})

		if err := api.SendEvents(ctx, rsAPI, api.KindNew, []*types.HeaderedEvent{bobMsg, charlieMsg}, "test", "test", "test", nil, false); err != nil {
			t.Errorf("failed to send events: %v", err)
		}

		// Bob changes his name
		expectedDisplayname := "Bob!"
		bobDisplayname := room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]any{"membership": "join", "displayname": expectedDisplayname}, test.WithStateKey(bob.ID))

		if err := api.SendEvents(ctx, rsAPI, api.KindNew, []*types.HeaderedEvent{bobDisplayname}, "test", "test", "test", nil, false); err != nil {
			t.Errorf("failed to send events: %v", err)
		}

		// Change another state event
		jrEv := room.CreateAndInsert(t, alice, spec.MRoomJoinRules, gomatrixserverlib.JoinRuleContent{JoinRule: "invite"}, test.WithStateKey(""))
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, []*types.HeaderedEvent{jrEv}, "test", "test", "test", nil, false); err != nil {
			t.Errorf("failed to send events: %v", err)
		}

		// send a message
		bobMsg = room.CreateAndInsert(t, bob, "m.room.message", map[string]any{"body": "hello world"})
		charlieMsg = room.CreateAndInsert(t, charlie, "m.room.message", map[string]any{"body": "hello world"})

		if err := api.SendEvents(ctx, rsAPI, api.KindNew, []*types.HeaderedEvent{bobMsg, charlieMsg}, "test", "test", "test", nil, false); err != nil {
			t.Errorf("failed to send events: %v", err)
		}

		// Craft the state reset message, which is using Bobs initial join event and the
		// last message Charlie sent as the prev_events. This should trigger the recalculation
		// of the "current" state, since the message event does not have state and no missing events in the DB.
		stateResetMsg := mustCreateEvent(t, fledglingEvent{
			Type:     "m.room.message",
			SenderID: charlie.ID,
			RoomID:   room.ID,
			Depth:    charlieMsg.Depth() + 1,
			PrevEvents: []any{
				bobJoinEv.EventID(),
				charlieMsg.EventID(),
			},
			AuthEvents: []any{
				room.Events()[0].EventID(), // create event
				room.Events()[2].EventID(), // PL event
				charlieJoinEv.EventID(),    // Charlie join event
			},
		})

		// Send the state reset message
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, []*types.HeaderedEvent{stateResetMsg}, "test", "test", "test", nil, false); err != nil {
			t.Errorf("failed to send events: %v", err)
		}

		// Validate that there is a membership event for Bob
		bobMembershipEv := api.GetStateEvent(ctx, rsAPI, room.ID, gomatrixserverlib.StateKeyTuple{
			EventType: spec.MRoomMember,
			StateKey:  bob.ID,
		})

		if bobMembershipEv == nil {
			t.Fatalf("Membership event for Bob does not exist. State reset?")
		} else {
			// Validate it's the correct membership event
			if dn := gjson.GetBytes(bobMembershipEv.Content(), "displayname").Str; dn != expectedDisplayname {
				t.Fatalf("Expected displayname to be %q, got %q", expectedDisplayname, dn)
			}
		}
	})
}

func TestNewServerACLs(t *testing.T) {
	alice := test.NewUser(t)
	roomWithACL := test.NewRoom(t, alice)

	roomWithACL.CreateAndInsert(t, alice, acls.MRoomServerACL, acls.ServerACL{
		Allowed:         []string{"*"},
		Denied:          []string{"localhost"},
		AllowIPLiterals: false,
	}, test.WithStateKey(""))

	roomWithoutACL := test.NewRoom(t, alice)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, closeDB := testrig.CreateConfig(t, dbType)
		defer closeDB()

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		natsInstance := &jetstream.NATSInstance{}
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		// start JetStream listeners
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)

		// let the RS create the events
		err := api.SendEvents(context.Background(), rsAPI, api.KindNew, roomWithACL.Events(), "test", "test", "test", nil, false)
		assert.NoError(t, err)
		err = api.SendEvents(context.Background(), rsAPI, api.KindNew, roomWithoutACL.Events(), "test", "test", "test", nil, false)
		assert.NoError(t, err)

		db, err := storage.Open(processCtx.Context(), cm, &cfg.RoomServer.Database, caches)
		assert.NoError(t, err)
		// create new server ACLs and verify server is banned/not banned
		serverACLs := acls.NewServerACLs(db)
		banned := serverACLs.IsServerBannedFromRoom("localhost", roomWithACL.ID)
		assert.Equal(t, true, banned)
		banned = serverACLs.IsServerBannedFromRoom("localhost", roomWithoutACL.ID)
		assert.Equal(t, false, banned)
	})
}

// Validate that changing the AckPolicy/AckWait of room consumers
// results in their recreation
func TestRoomConsumerRecreation(t *testing.T) {

	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)

	// As this is DB unrelated, just use SQLite
	cfg, processCtx, closeDB := testrig.CreateConfig(t, test.DBTypeSQLite)
	defer closeDB()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	natsInstance := &jetstream.NATSInstance{}

	// Prepare a stream and consumer using the old configuration
	jsCtx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)

	streamName := cfg.Global.JetStream.Prefixed(jetstream.InputRoomEvent)
	consumer := cfg.Global.JetStream.Prefixed("RoomInput" + jetstream.Tokenise(room.ID))
	subject := cfg.Global.JetStream.Prefixed(jetstream.InputRoomEventSubj(room.ID))

	consumerConfig := &nats.ConsumerConfig{
		Durable:           consumer,
		AckPolicy:         nats.AckAllPolicy,
		DeliverPolicy:     nats.DeliverAllPolicy,
		FilterSubject:     subject,
		AckWait:           (time.Minute * 2) + (time.Second * 10),
		InactiveThreshold: time.Hour * 24,
	}

	// Create the consumer with the old config
	_, err := jsCtx.AddConsumer(streamName, consumerConfig)
	assert.NoError(t, err)

	caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
	// start JetStream listeners
	rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)
	rsAPI.SetFederationAPI(nil, nil)

	// let the RS create the events, this also recreates the Consumers
	err = api.SendEvents(context.Background(), rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false)
	assert.NoError(t, err)

	// Validate that AckPolicy and AckWait has changed
	info, err := jsCtx.ConsumerInfo(streamName, consumer)
	assert.NoError(t, err)
	assert.Equal(t, nats.AckExplicitPolicy, info.Config.AckPolicy)

	wantAckWait := input.MaximumMissingProcessingTime + (time.Second * 10)
	assert.Equal(t, wantAckWait, info.Config.AckWait)
}

func TestRoomsWithACLs(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser(t)
	noACLRoom := test.NewRoom(t, alice)
	aclRoom := test.NewRoom(t, alice)

	aclRoom.CreateAndInsert(t, alice, "m.room.server_acl", map[string]any{
		"deny":  []string{"evilhost.test"},
		"allow": []string{"*"},
	}, test.WithStateKey(""))

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, closeDB := testrig.CreateConfig(t, dbType)
		defer closeDB()

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		natsInstance := &jetstream.NATSInstance{}
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		// start JetStream listeners
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)

		for _, room := range []*test.Room{noACLRoom, aclRoom} {
			// Create the rooms
			err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false)
			assert.NoError(t, err)
		}

		// Validate that we only have one ACLd room.
		roomsWithACLs, err := rsAPI.RoomsWithACLs(ctx)
		assert.NoError(t, err)
		assert.Equal(t, []string{aclRoom.ID}, roomsWithACLs)
	})
}
