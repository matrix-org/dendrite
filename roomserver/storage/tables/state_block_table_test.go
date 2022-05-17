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
	"github.com/stretchr/testify/assert"
)

func mustCreateStateBlockTable(t *testing.T, dbType test.DBType) (tab tables.StateBlock, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateStateBlockTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareStateBlockTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateStateBlockTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareStateBlockTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func TestStateBlockTable(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateStateBlockTable(t, dbType)
		defer close()

		// generate some dummy data
		var entries types.StateEntries
		for i := 0; i < 100; i++ {
			entry := types.StateEntry{
				EventNID: types.EventNID(i),
			}
			entries = append(entries, entry)
		}
		stateBlockNID, err := tab.BulkInsertStateData(ctx, nil, entries)
		assert.NoError(t, err)
		assert.Equal(t, types.StateBlockNID(1), stateBlockNID)

		// generate a different hash, to get a new StateBlockNID
		var entries2 types.StateEntries
		for i := 100; i < 300; i++ {
			entry := types.StateEntry{
				EventNID: types.EventNID(i),
			}
			entries2 = append(entries2, entry)
		}
		stateBlockNID, err = tab.BulkInsertStateData(ctx, nil, entries2)
		assert.NoError(t, err)
		assert.Equal(t, types.StateBlockNID(2), stateBlockNID)

		eventNIDs, err := tab.BulkSelectStateBlockEntries(ctx, nil, types.StateBlockNIDs{1, 2})
		assert.NoError(t, err)
		assert.Equal(t, len(entries), len(eventNIDs[0]))
		assert.Equal(t, len(entries2), len(eventNIDs[1]))

		// try to get a StateBlockNID which does not exist
		_, err = tab.BulkSelectStateBlockEntries(ctx, nil, types.StateBlockNIDs{5})
		assert.Error(t, err)

		// This should return an error, since we can only retrieve 1 StateBlock
		_, err = tab.BulkSelectStateBlockEntries(ctx, nil, types.StateBlockNIDs{1, 5})
		assert.Error(t, err)

		for i := 0; i < 65555; i++ {
			entry := types.StateEntry{
				EventNID: types.EventNID(i),
			}
			entries2 = append(entries2, entry)
		}
		stateBlockNID, err = tab.BulkInsertStateData(ctx, nil, entries2)
		assert.NoError(t, err)
		assert.Equal(t, types.StateBlockNID(3), stateBlockNID)
	})
}
