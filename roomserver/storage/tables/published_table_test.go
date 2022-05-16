package tables_test

import (
	"context"
	"sort"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/stretchr/testify/assert"
)

func mustCreatePublishedTable(t *testing.T, dbType test.DBType) (tab tables.Published, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreatePublishedTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PreparePublishedTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreatePublishedTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PreparePublishedTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func TestPublishedTable(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser(t)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreatePublishedTable(t, dbType)
		defer close()

		// Publish some rooms
		publishedRooms := []string{}
		for i := 0; i < 10; i++ {
			room := test.NewRoom(t, alice)
			published := i%2 == 0
			err := tab.UpsertRoomPublished(ctx, nil, room.ID, published)
			assert.NoError(t, err)
			if published {
				publishedRooms = append(publishedRooms, room.ID)
			}
			publishedRes, err := tab.SelectPublishedFromRoomID(ctx, nil, room.ID)
			assert.NoError(t, err)
			assert.Equal(t, published, publishedRes)
		}
		sort.Strings(publishedRooms)

		// check that we get the expected published rooms
		roomIDs, err := tab.SelectAllPublishedRooms(ctx, nil, true)
		assert.NoError(t, err)
		assert.Equal(t, publishedRooms, roomIDs)

		// test an actual upsert
		room := test.NewRoom(t, alice)
		err = tab.UpsertRoomPublished(ctx, nil, room.ID, true)
		assert.NoError(t, err)
		err = tab.UpsertRoomPublished(ctx, nil, room.ID, false)
		assert.NoError(t, err)
		// should now be false, due to the upsert
		publishedRes, err := tab.SelectPublishedFromRoomID(ctx, nil, room.ID)
		assert.NoError(t, err)
		assert.False(t, publishedRes)
	})
}
