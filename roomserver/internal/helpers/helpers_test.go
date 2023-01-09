package helpers

import (
	"context"
	"testing"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/stretchr/testify/assert"

	"github.com/matrix-org/dendrite/setup/base"

	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"

	"github.com/matrix-org/dendrite/roomserver/storage"
)

func mustCreateDatabase(t *testing.T, dbType test.DBType) (*base.BaseDendrite, storage.Database, func()) {
	base, close := testrig.CreateBaseDendrite(t, dbType)
	db, err := storage.Open(base, &base.Cfg.RoomServer.Database, base.Caches)
	if err != nil {
		t.Fatalf("failed to create Database: %v", err)
	}
	return base, db, close
}

func TestIsInvitePendingWithoutNID(t *testing.T) {

	alice := test.NewUser(t)
	bob := test.NewUser(t)
	room := test.NewRoom(t, alice, test.RoomPreset(test.PresetPublicChat))
	_ = bob
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		_, db, close := mustCreateDatabase(t, dbType)
		defer close()

		// store all events
		var authNIDs []types.EventNID
		for _, x := range room.Events() {

			evNID, _, _, _, _, err := db.StoreEvent(context.Background(), x.Event, authNIDs, false)
			assert.NoError(t, err)
			authNIDs = append(authNIDs, evNID)
		}

		// Alice should have no pending invites and should have a NID
		pendingInvite, _, _, _, err := IsInvitePending(context.Background(), db, room.ID, alice.ID)
		assert.NoError(t, err, "failed to get pending invites")
		assert.False(t, pendingInvite, "unexpected pending invite")

		// Bob should have no pending invites and receive a new NID
		pendingInvite, _, _, _, err = IsInvitePending(context.Background(), db, room.ID, bob.ID)
		assert.NoError(t, err, "failed to get pending invites")
		assert.False(t, pendingInvite, "unexpected pending invite")
	})
}
