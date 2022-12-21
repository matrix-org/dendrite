package roomserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/syncapi"
	"github.com/matrix-org/dendrite/userapi"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/inthttp"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
)

func mustCreateDatabase(t *testing.T, dbType test.DBType) (*base.BaseDendrite, storage.Database, func()) {
	t.Helper()
	base, close := testrig.CreateBaseDendrite(t, dbType)
	db, err := storage.Open(base, &base.Cfg.RoomServer.Database, base.Caches)
	if err != nil {
		t.Fatalf("failed to create Database: %v", err)
	}
	return base, db, close
}

func Test_SharedUsers(t *testing.T) {
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
		base, _, close := mustCreateDatabase(t, dbType)
		defer close()

		rsAPI := roomserver.NewInternalAPI(base)
		// SetFederationAPI starts the room event input consumer
		rsAPI.SetFederationAPI(nil, nil)
		// Create the room
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatalf("failed to send events: %v", err)
		}

		// Query the shared users for Alice, there should only be Bob.
		// This is used by the SyncAPI keychange consumer.
		res := &api.QuerySharedUsersResponse{}
		if err := rsAPI.QuerySharedUsers(ctx, &api.QuerySharedUsersRequest{UserID: alice.ID}, res); err != nil {
			t.Fatalf("unable to query known users: %v", err)
		}
		if _, ok := res.UserIDsToCount[bob.ID]; !ok {
			t.Fatalf("expected to find %s in shared users, but didn't: %+v", bob.ID, res.UserIDsToCount)
		}
		// Also verify that we get the expected result when specifying OtherUserIDs.
		// This is used by the SyncAPI when getting device list changes.
		if err := rsAPI.QuerySharedUsers(ctx, &api.QuerySharedUsersRequest{UserID: alice.ID, OtherUserIDs: []string{bob.ID}}, res); err != nil {
			t.Fatalf("unable to query known users: %v", err)
		}
		if _, ok := res.UserIDsToCount[bob.ID]; !ok {
			t.Fatalf("expected to find %s in shared users, but didn't: %+v", bob.ID, res.UserIDsToCount)
		}
	})
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
		base, _, close := mustCreateDatabase(t, dbType)
		defer close()

		rsAPI := roomserver.NewInternalAPI(base)
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

		t.Run("HTTP API", func(t *testing.T) {
			router := mux.NewRouter().PathPrefix(httputil.InternalPathPrefix).Subrouter()
			roomserver.AddInternalRoutes(router, rsAPI, false)
			apiURL, cancel := test.ListenAndServe(t, router, false)
			defer cancel()
			httpAPI, err := inthttp.NewRoomserverClient(apiURL, &http.Client{Timeout: time.Second * 5}, nil)
			if err != nil {
				t.Fatalf("failed to create HTTP client")
			}
			testCase(httpAPI)
		})
		t.Run("Monolith", func(t *testing.T) {
			testCase(rsAPI)
			// also test tracing
			traceAPI := &api.RoomserverInternalAPITrace{Impl: rsAPI}
			testCase(traceAPI)
		})

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
		if dbType == test.DBTypeSQLite {
			t.Skip("purging rooms on SQLite is not yet implemented")
		}
		base, db, close := mustCreateDatabase(t, dbType)
		defer close()

		fedClient := base.CreateFederationClient()
		rsAPI := roomserver.NewInternalAPI(base)
		keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, fedClient, rsAPI)
		userAPI := userapi.NewInternalAPI(base, &base.Cfg.UserAPI, nil, keyAPI, rsAPI, nil)

		// this starts the JetStream consumers
		syncapi.AddPublicRoutes(base, userAPI, rsAPI, keyAPI)
		federationapi.NewInternalAPI(base, fedClient, rsAPI, base.Caches, nil, true)
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
		// TODO: Find a better solution, e.g. "hook" into the JetStream stream.
		//  Gives the SyncAPI and FederationAPI some time to delete their entries
		time.Sleep(time.Millisecond * 100)

		roomInfo, err = db.RoomInfo(ctx, room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if roomInfo != nil {
			t.Fatalf("room should not exist after purging: %+v", roomInfo)
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
