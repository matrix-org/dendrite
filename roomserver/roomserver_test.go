package roomserver_test

import (
	"context"
	"crypto/ed25519"
	"reflect"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/stretchr/testify/assert"

	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/userapi"

	userAPI "github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/syncapi"

	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
)

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
			usrAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
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
	room.CreateAndInsert(t, alice, gomatrixserverlib.MRoomMember, map[string]interface{}{
		"membership": "invite",
	}, test.WithStateKey(bob.ID))
	room.CreateAndInsert(t, bob, gomatrixserverlib.MRoomMember, map[string]interface{}{
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
	room.CreateAndInsert(t, bob, gomatrixserverlib.MRoomMember, map[string]interface{}{
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
	revokeEvent := room.CreateAndInsert(t, alice, gomatrixserverlib.MRoomGuestAccess, map[string]string{"guest_access": "forbidden"}, test.WithStateKey(""))
	if err := api.SendEvents(ctx, rsAPI, api.KindNew, []*gomatrixserverlib.HeaderedEvent{revokeEvent}, "test", "test", "test", nil, false); err != nil {
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
	room.CreateAndInsert(t, alice, gomatrixserverlib.MRoomMember, map[string]interface{}{
		"membership": "invite",
	}, test.WithStateKey(bob.ID))
	room.CreateAndInsert(t, bob, gomatrixserverlib.MRoomMember, map[string]interface{}{
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

	// Invite Bob
	inviteEvent := room.CreateAndInsert(t, alice, gomatrixserverlib.MRoomMember, map[string]interface{}{
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

		jsCtx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
		defer jetstream.DeleteAllStreams(jsCtx, &cfg.Global.JetStream)

		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)

		// this starts the JetStream consumers
		syncapi.AddPublicRoutes(processCtx, routers, cfg, cm, &natsInstance, userAPI, rsAPI, caches, caching.DisableMetrics)
		federationapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, nil, rsAPI, caches, nil, true)
		rsAPI.SetFederationAPI(nil, nil)

		// Create the room
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatalf("failed to send events: %v", err)
		}

		// some dummy entries to validate after purging
		publishResp := &api.PerformPublishResponse{}
		if err := rsAPI.PerformPublish(ctx, &api.PerformPublishRequest{RoomID: room.ID, Visibility: "public"}, publishResp); err != nil {
			t.Fatal(err)
		}
		if publishResp.Error != nil {
			t.Fatal(publishResp.Error)
		}

		isPublished, err := db.GetPublishedRoom(ctx, room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !isPublished {
			t.Fatalf("room should be published before purging")
		}

		aliasResp := &api.SetRoomAliasResponse{}
		if err = rsAPI.SetRoomAlias(ctx, &api.SetRoomAliasRequest{RoomID: room.ID, Alias: "myalias", UserID: alice.ID}, aliasResp); err != nil {
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
		purgeResp := &api.PerformAdminPurgeRoomResponse{}
		if err = rsAPI.PerformAdminPurgeRoom(ctx, &api.PerformAdminPurgeRoomRequest{RoomID: room.ID}, purgeResp); err != nil {
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
	Sender     string
	RoomID     string
	Redacts    string
	Depth      int64
	PrevEvents []interface{}
}

func mustCreateEvent(t *testing.T, ev fledglingEvent) (result *gomatrixserverlib.HeaderedEvent) {
	t.Helper()
	roomVer := gomatrixserverlib.RoomVersionV9
	seed := make([]byte, ed25519.SeedSize) // zero seed
	key := ed25519.NewKeyFromSeed(seed)
	eb := gomatrixserverlib.EventBuilder{
		Sender:     ev.Sender,
		Type:       ev.Type,
		StateKey:   ev.StateKey,
		RoomID:     ev.RoomID,
		Redacts:    ev.Redacts,
		Depth:      ev.Depth,
		PrevEvents: ev.PrevEvents,
	}
	err := eb.SetContent(map[string]interface{}{})
	if err != nil {
		t.Fatalf("mustCreateEvent: failed to marshal event content %v", err)
	}
	signedEvent, err := eb.Build(time.Now(), "localhost", "ed25519:test", key, roomVer)
	if err != nil {
		t.Fatalf("mustCreateEvent: failed to sign event: %s", err)
	}
	h := signedEvent.Headered(roomVer)
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
					Type:       gomatrixserverlib.MRoomRedaction,
					Sender:     alice.ID,
					RoomID:     room.ID,
					Redacts:    redactedEvent.EventID(),
					Depth:      redactedEvent.Depth() + 1,
					PrevEvents: []interface{}{redactedEvent.EventID()},
				})
				room.InsertEvent(t, builderEv.Headered(gomatrixserverlib.RoomVersionV9))
			},
		},
		{
			name:         "can redact others message, allowed by PL",
			wantRedacted: true,
			additionalEvents: func(t *testing.T, room *test.Room) {
				redactedEvent := room.CreateAndInsert(t, bob, "m.room.message", map[string]interface{}{"body": "hello world"})

				builderEv := mustCreateEvent(t, fledglingEvent{
					Type:       gomatrixserverlib.MRoomRedaction,
					Sender:     alice.ID,
					RoomID:     room.ID,
					Redacts:    redactedEvent.EventID(),
					Depth:      redactedEvent.Depth() + 1,
					PrevEvents: []interface{}{redactedEvent.EventID()},
				})
				room.InsertEvent(t, builderEv.Headered(gomatrixserverlib.RoomVersionV9))
			},
		},
		{
			name:         "can redact others message, same server",
			wantRedacted: true,
			additionalEvents: func(t *testing.T, room *test.Room) {
				redactedEvent := room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": "hello world"})

				builderEv := mustCreateEvent(t, fledglingEvent{
					Type:       gomatrixserverlib.MRoomRedaction,
					Sender:     bob.ID,
					RoomID:     room.ID,
					Redacts:    redactedEvent.EventID(),
					Depth:      redactedEvent.Depth() + 1,
					PrevEvents: []interface{}{redactedEvent.EventID()},
				})
				room.InsertEvent(t, builderEv.Headered(gomatrixserverlib.RoomVersionV9))
			},
		},
		{
			name: "can not redact others message, missing PL",
			additionalEvents: func(t *testing.T, room *test.Room) {
				redactedEvent := room.CreateAndInsert(t, bob, "m.room.message", map[string]interface{}{"body": "hello world"})

				builderEv := mustCreateEvent(t, fledglingEvent{
					Type:       gomatrixserverlib.MRoomRedaction,
					Sender:     charlie.ID,
					RoomID:     room.ID,
					Redacts:    redactedEvent.EventID(),
					Depth:      redactedEvent.Depth() + 1,
					PrevEvents: []interface{}{redactedEvent.EventID()},
				})
				room.InsertEvent(t, builderEv.Headered(gomatrixserverlib.RoomVersionV9))
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

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				authEvents := []types.EventNID{}
				var roomInfo *types.RoomInfo
				var err error

				room := test.NewRoom(t, alice, test.RoomPreset(test.PresetPublicChat))
				room.CreateAndInsert(t, bob, gomatrixserverlib.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, charlie, gomatrixserverlib.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(charlie.ID))

				if tc.additionalEvents != nil {
					tc.additionalEvents(t, room)
				}

				for _, ev := range room.Events() {
					roomInfo, err = db.GetOrCreateRoomInfo(ctx, ev.Event)
					assert.NoError(t, err)
					assert.NotNil(t, roomInfo)
					evTypeNID, err := db.GetOrCreateEventTypeNID(ctx, ev.Type())
					assert.NoError(t, err)

					stateKeyNID, err := db.GetOrCreateEventStateKeyNID(ctx, ev.StateKey())
					assert.NoError(t, err)

					eventNID, stateAtEvent, err := db.StoreEvent(ctx, ev.Event, roomInfo, evTypeNID, stateKeyNID, authEvents, false)
					assert.NoError(t, err)
					if ev.StateKey() != nil {
						authEvents = append(authEvents, eventNID)
					}

					// Calculate the snapshotNID etc.
					plResolver := state.NewStateResolution(db, roomInfo)
					stateAtEvent.BeforeStateSnapshotNID, err = plResolver.CalculateAndStoreStateBeforeEvent(ctx, ev.Event, false)
					assert.NoError(t, err)

					// Update the room
					updater, err := db.GetRoomUpdater(ctx, roomInfo)
					assert.NoError(t, err)
					err = updater.SetState(ctx, eventNID, stateAtEvent.BeforeStateSnapshotNID)
					assert.NoError(t, err)
					err = updater.Commit()
					assert.NoError(t, err)

					_, redactedEvent, err := db.MaybeRedactEvent(ctx, roomInfo, eventNID, ev.Event, &plResolver)
					assert.NoError(t, err)
					if redactedEvent != nil {
						assert.Equal(t, ev.Redacts(), redactedEvent.EventID())
					}
					if ev.Type() == gomatrixserverlib.MRoomRedaction {
						nids, err := db.EventNIDs(ctx, []string{ev.Redacts()})
						assert.NoError(t, err)
						evs, err := db.Events(ctx, roomInfo, []types.EventNID{nids[ev.Redacts()].EventNID})
						assert.NoError(t, err)
						assert.Equal(t, 1, len(evs))
						assert.Equal(t, tc.wantRedacted, evs[0].Redacted())
					}
				}
			})
		}
	})
}
