package helpers

import (
	"context"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/stretchr/testify/assert"

	"github.com/matrix-org/dendrite/roomserver/types"

	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/test"
)

func mustCreateDatabase(t *testing.T, dbType test.DBType) (storage.Database, func()) {
	conStr, close := test.PrepareDBConnectionString(t, dbType)
	caches := caching.NewRistrettoCache(8*1024*1024, time.Hour, caching.DisableMetrics)
	cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})
	db, err := storage.Open(context.Background(), cm, &config.DatabaseOptions{ConnectionString: config.DataSource(conStr)}, caches)
	if err != nil {
		t.Fatalf("failed to create Database: %v", err)
	}
	return db, close
}

func TestIsInvitePendingWithoutNID(t *testing.T) {

	alice := test.NewUser(t)
	bob := test.NewUser(t)
	room := test.NewRoom(t, alice, test.RoomPreset(test.PresetPublicChat))
	_ = bob
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		// store all events
		var authNIDs []types.EventNID
		for _, x := range room.Events() {

			roomInfo, err := db.GetOrCreateRoomInfo(context.Background(), x.PDU)
			assert.NoError(t, err)
			assert.NotNil(t, roomInfo)

			eventTypeNID, err := db.GetOrCreateEventTypeNID(context.Background(), x.Type())
			assert.NoError(t, err)
			assert.Greater(t, eventTypeNID, types.EventTypeNID(0))

			eventStateKeyNID, err := db.GetOrCreateEventStateKeyNID(context.Background(), x.StateKey())
			assert.NoError(t, err)

			evNID, _, err := db.StoreEvent(context.Background(), x.PDU, roomInfo, eventTypeNID, eventStateKeyNID, authNIDs, false)
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
