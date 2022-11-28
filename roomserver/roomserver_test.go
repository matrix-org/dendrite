package roomserver_test

import (
	"context"
	"testing"

	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/gomatrixserverlib"
)

func mustCreateDatabase(t *testing.T, dbType test.DBType) (*base.BaseDendrite, storage.Database, func()) {
	base, close := testrig.CreateBaseDendrite(t, dbType)
	db, err := storage.Open(base, &base.Cfg.KeyServer.Database, base.Caches)
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
