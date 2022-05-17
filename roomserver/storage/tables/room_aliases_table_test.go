package tables_test

import (
	"context"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/stretchr/testify/assert"
)

func mustCreateRoomAliasesTable(t *testing.T, dbType test.DBType) (tab tables.RoomAliases, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateRoomAliasesTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareRoomAliasesTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateRoomAliasesTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareRoomAliasesTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func TestRoomAliasesTable(t *testing.T) {
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)
	room2 := test.NewRoom(t, alice)
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateRoomAliasesTable(t, dbType)
		defer close()
		alias, alias2, alias3 := "#alias:localhost", "#alias2:localhost", "#alias3:localhost"
		// insert aliases
		err := tab.InsertRoomAlias(ctx, nil, alias, room.ID, alice.ID)
		assert.NoError(t, err)

		err = tab.InsertRoomAlias(ctx, nil, alias2, room.ID, alice.ID)
		assert.NoError(t, err)

		err = tab.InsertRoomAlias(ctx, nil, alias3, room2.ID, alice.ID)
		assert.NoError(t, err)

		// verify we can get the roomID for the alias
		roomID, err := tab.SelectRoomIDFromAlias(ctx, nil, alias)
		assert.NoError(t, err)
		assert.Equal(t, room.ID, roomID)

		// .. and the creator
		creator, err := tab.SelectCreatorIDFromAlias(ctx, nil, alias)
		assert.NoError(t, err)
		assert.Equal(t, alice.ID, creator)

		creator, err = tab.SelectCreatorIDFromAlias(ctx, nil, "#doesntexist:localhost")
		assert.NoError(t, err)
		assert.Equal(t, "", creator)

		roomID, err = tab.SelectRoomIDFromAlias(ctx, nil, "#doesntexist:localhost")
		assert.NoError(t, err)
		assert.Equal(t, "", roomID)

		// get all aliases for a room
		aliases, err := tab.SelectAliasesFromRoomID(ctx, nil, room.ID)
		assert.NoError(t, err)
		assert.Equal(t, []string{alias, alias2}, aliases)

		// delete an alias and verify it's deleted
		err = tab.DeleteRoomAlias(ctx, nil, alias2)
		assert.NoError(t, err)

		aliases, err = tab.SelectAliasesFromRoomID(ctx, nil, room.ID)
		assert.NoError(t, err)
		assert.Equal(t, []string{alias}, aliases)

		// deleting the same alias should be a no-op
		err = tab.DeleteRoomAlias(ctx, nil, alias2)
		assert.NoError(t, err)

		// Delete non-existent alias should be a no-op
		err = tab.DeleteRoomAlias(ctx, nil, "#doesntexist:localhost")
		assert.NoError(t, err)
	})
}
