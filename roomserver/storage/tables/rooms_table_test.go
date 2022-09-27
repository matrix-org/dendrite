package tables_test

import (
	"context"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/util"
	"github.com/stretchr/testify/assert"
)

func mustCreateRoomsTable(t *testing.T, dbType test.DBType) (tab tables.Rooms, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateRoomsTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareRoomsTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateRoomsTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareRoomsTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func TestRoomsTable(t *testing.T) {
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateRoomsTable(t, dbType)
		defer close()

		wantRoomNID, err := tab.InsertRoomNID(ctx, nil, room.ID, room.Version)
		assert.NoError(t, err)

		// Create dummy room
		_, err = tab.InsertRoomNID(ctx, nil, util.RandomString(16), room.Version)
		assert.NoError(t, err)

		gotRoomNID, err := tab.SelectRoomNID(ctx, nil, room.ID)
		assert.NoError(t, err)
		assert.Equal(t, wantRoomNID, gotRoomNID)

		// Ensure non existent roomNID errors
		roomNID, err := tab.SelectRoomNID(ctx, nil, "!doesnotexist:localhost")
		assert.Error(t, err)
		assert.Equal(t, types.RoomNID(0), roomNID)

		roomInfo, err := tab.SelectRoomInfo(ctx, nil, room.ID)
		assert.NoError(t, err)
		expected := &types.RoomInfo{
			RoomNID:     wantRoomNID,
			RoomVersion: room.Version,
		}
		expected.SetIsStub(true) // there are no latestEventNIDs
		assert.Equal(t, expected, roomInfo)

		roomInfo, err = tab.SelectRoomInfo(ctx, nil, "!doesnotexist:localhost")
		assert.NoError(t, err)
		assert.Nil(t, roomInfo)

		// There are no rooms with latestEventNIDs yet
		roomIDs, err := tab.SelectRoomIDsWithEvents(ctx, nil)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(roomIDs))

		roomVersions, err := tab.SelectRoomVersionsForRoomNIDs(ctx, nil, []types.RoomNID{wantRoomNID, 1337})
		assert.NoError(t, err)
		assert.Equal(t, roomVersions[wantRoomNID], room.Version)
		// Room does not exist
		_, ok := roomVersions[1337]
		assert.False(t, ok)

		roomIDs, err = tab.BulkSelectRoomIDs(ctx, nil, []types.RoomNID{wantRoomNID, 1337})
		assert.NoError(t, err)
		assert.Equal(t, []string{room.ID}, roomIDs)

		roomNIDs, err := tab.BulkSelectRoomNIDs(ctx, nil, []string{room.ID, "!doesnotexist:localhost"})
		assert.NoError(t, err)
		assert.Equal(t, []types.RoomNID{wantRoomNID}, roomNIDs)

		wantEventNIDs := []types.EventNID{1, 2, 3}
		lastEventSentNID := types.EventNID(3)
		stateSnapshotNID := types.StateSnapshotNID(1)
		// make the room "usable"
		err = tab.UpdateLatestEventNIDs(ctx, nil, wantRoomNID, wantEventNIDs, lastEventSentNID, stateSnapshotNID)
		assert.NoError(t, err)

		roomInfo, err = tab.SelectRoomInfo(ctx, nil, room.ID)
		assert.NoError(t, err)
		expected = &types.RoomInfo{
			RoomNID:     wantRoomNID,
			RoomVersion: room.Version,
		}
		expected.SetStateSnapshotNID(1)
		assert.Equal(t, expected, roomInfo)

		eventNIDs, snapshotNID, err := tab.SelectLatestEventNIDs(ctx, nil, wantRoomNID)
		assert.NoError(t, err)
		assert.Equal(t, wantEventNIDs, eventNIDs)
		assert.Equal(t, types.StateSnapshotNID(1), snapshotNID)

		// Again, doesn't exist
		_, _, err = tab.SelectLatestEventNIDs(ctx, nil, 1337)
		assert.Error(t, err)

		eventNIDs, eventNID, snapshotNID, err := tab.SelectLatestEventsNIDsForUpdate(ctx, nil, wantRoomNID)
		assert.NoError(t, err)
		assert.Equal(t, wantEventNIDs, eventNIDs)
		assert.Equal(t, types.EventNID(3), eventNID)
		assert.Equal(t, types.StateSnapshotNID(1), snapshotNID)
	})
}
