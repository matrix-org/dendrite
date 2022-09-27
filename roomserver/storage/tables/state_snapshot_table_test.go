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

func mustCreateStateSnapshotTable(t *testing.T, dbType test.DBType) (tab tables.StateSnapshot, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	switch dbType {
	case test.DBTypePostgres:
		// for the PostgreSQL history visibility optimisation to work,
		// we also need some other tables to exist
		err = postgres.CreateEventStateKeysTable(db)
		assert.NoError(t, err)
		err = postgres.CreateEventsTable(db)
		assert.NoError(t, err)
		err = postgres.CreateStateBlockTable(db)
		assert.NoError(t, err)
		// ... and then the snapshot table itself
		err = postgres.CreateStateSnapshotTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareStateSnapshotTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateStateSnapshotTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareStateSnapshotTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func TestStateSnapshotTable(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateStateSnapshotTable(t, dbType)
		defer close()

		// generate some dummy data
		var stateBlockNIDs types.StateBlockNIDs
		for i := 0; i < 100; i++ {
			stateBlockNIDs = append(stateBlockNIDs, types.StateBlockNID(i))
		}
		stateNID, err := tab.InsertState(ctx, nil, 1, stateBlockNIDs)
		assert.NoError(t, err)
		assert.Equal(t, types.StateSnapshotNID(1), stateNID)

		// verify ON CONFLICT; Note: this updates the sequence!
		stateNID, err = tab.InsertState(ctx, nil, 1, stateBlockNIDs)
		assert.NoError(t, err)
		assert.Equal(t, types.StateSnapshotNID(1), stateNID)

		// create a second snapshot
		var stateBlockNIDs2 types.StateBlockNIDs
		for i := 100; i < 150; i++ {
			stateBlockNIDs2 = append(stateBlockNIDs2, types.StateBlockNID(i))
		}

		stateNID, err = tab.InsertState(ctx, nil, 1, stateBlockNIDs2)
		assert.NoError(t, err)
		// StateSnapshotNID is now 3, since the DO UPDATE SET statement incremented the sequence
		assert.Equal(t, types.StateSnapshotNID(3), stateNID)

		nidLists, err := tab.BulkSelectStateBlockNIDs(ctx, nil, []types.StateSnapshotNID{1, 3})
		assert.NoError(t, err)
		assert.Equal(t, stateBlockNIDs, types.StateBlockNIDs(nidLists[0].StateBlockNIDs))
		assert.Equal(t, stateBlockNIDs2, types.StateBlockNIDs(nidLists[1].StateBlockNIDs))

		// check we get an error if the state snapshot does not exist
		_, err = tab.BulkSelectStateBlockNIDs(ctx, nil, []types.StateSnapshotNID{2})
		assert.Error(t, err)

		// create a second snapshot
		for i := 0; i < 65555; i++ {
			stateBlockNIDs2 = append(stateBlockNIDs2, types.StateBlockNID(i))
		}
		_, err = tab.InsertState(ctx, nil, 1, stateBlockNIDs2)
		assert.NoError(t, err)
	})
}
