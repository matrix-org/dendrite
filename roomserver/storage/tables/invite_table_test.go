package tables_test

import (
	"context"
	"testing"

	"github.com/matrix-org/util"
	"github.com/stretchr/testify/assert"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
)

func mustCreateInviteTable(t *testing.T, dbType test.DBType) (tables.Invites, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.Invites
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateInvitesTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareInvitesTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateInvitesTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareInvitesTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func TestInviteTable(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateInviteTable(t, dbType)
		defer close()
		eventID1 := util.RandomString(16)
		roomNID := types.RoomNID(1)
		targetUserNID, senderUserNID := types.EventStateKeyNID(1), types.EventStateKeyNID(2)
		newInvite, err := tab.InsertInviteEvent(ctx, nil, eventID1, roomNID, targetUserNID, senderUserNID, []byte(""))
		assert.NoError(t, err)
		assert.True(t, newInvite)

		// Try adding the same invite again
		newInvite, err = tab.InsertInviteEvent(ctx, nil, eventID1, roomNID, targetUserNID, senderUserNID, []byte(""))
		assert.NoError(t, err)
		assert.False(t, newInvite)

		// Add another invite for this room
		eventID2 := util.RandomString(16)
		newInvite, err = tab.InsertInviteEvent(ctx, nil, eventID2, roomNID, targetUserNID, senderUserNID, []byte(""))
		assert.NoError(t, err)
		assert.True(t, newInvite)

		// Add another invite for a different user
		eventID := util.RandomString(16)
		newInvite, err = tab.InsertInviteEvent(ctx, nil, eventID, types.RoomNID(3), targetUserNID, senderUserNID, []byte(""))
		assert.NoError(t, err)
		assert.True(t, newInvite)

		stateKeyNIDs, eventIDs, _, err := tab.SelectInviteActiveForUserInRoom(ctx, nil, targetUserNID, roomNID)
		assert.NoError(t, err)
		assert.Equal(t, []string{eventID1, eventID2}, eventIDs)
		assert.Equal(t, []types.EventStateKeyNID{2, 2}, stateKeyNIDs)

		// retire the invite
		retiredEventIDs, err := tab.UpdateInviteRetired(ctx, nil, roomNID, targetUserNID)
		assert.NoError(t, err)
		assert.Equal(t, []string{eventID1, eventID2}, retiredEventIDs)

		// This should now be empty
		stateKeyNIDs, eventIDs, _, err = tab.SelectInviteActiveForUserInRoom(ctx, nil, targetUserNID, roomNID)
		assert.NoError(t, err)
		assert.Empty(t, eventIDs)
		assert.Empty(t, stateKeyNIDs)

		// Non-existent targetUserNID
		stateKeyNIDs, eventIDs, _, err = tab.SelectInviteActiveForUserInRoom(ctx, nil, types.EventStateKeyNID(10), roomNID)
		assert.NoError(t, err)
		assert.Empty(t, stateKeyNIDs)
		assert.Empty(t, eventIDs)
	})
}
