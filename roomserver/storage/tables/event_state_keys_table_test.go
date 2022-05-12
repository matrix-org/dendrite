package tables_test

import (
	"context"
	"fmt"
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

func mustCreateEventStateKeysTable(t *testing.T, dbType test.DBType) (tables.EventStateKeys, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.EventStateKeys
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateEventStateKeysTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareEventStateKeysTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateEventStateKeysTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareEventStateKeysTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func Test_EventStateKeysTable(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateEventStateKeysTable(t, dbType)
		defer close()
		ctx := context.Background()
		var stateKeyNID, gotEventStateKey types.EventStateKeyNID
		var err error
		// create some dummy data
		for i := 0; i < 10; i++ {
			stateKey := fmt.Sprintf("@user%d:localhost", i)
			stateKeyNID, err = tab.InsertEventStateKeyNID(ctx, nil, stateKey)
			assert.NoError(t, err)
			gotEventStateKey, err = tab.SelectEventStateKeyNID(ctx, nil, stateKey)
			assert.NoError(t, err)
			assert.Equal(t, stateKeyNID, gotEventStateKey)
		}
		// This should fail, since @user0:localhost already exists
		stateKey := fmt.Sprintf("@user%d:localhost", 0)
		_, err = tab.InsertEventStateKeyNID(ctx, nil, stateKey)
		assert.Error(t, err)

		stateKeyNIDsMap, err := tab.BulkSelectEventStateKeyNID(ctx, nil, []string{"@user0:localhost", "@user1:localhost"})
		assert.NoError(t, err)
		wantStateKeyNIDs := make([]types.EventStateKeyNID, 0, len(stateKeyNIDsMap))
		for _, nid := range stateKeyNIDsMap {
			wantStateKeyNIDs = append(wantStateKeyNIDs, nid)
		}
		stateKeyNIDs, err := tab.BulkSelectEventStateKey(ctx, nil, wantStateKeyNIDs)
		assert.NoError(t, err)
		// verify that BulkSelectEventStateKeyNID and BulkSelectEventStateKey return the same values
		for userID, nid := range stateKeyNIDsMap {
			if v, ok := stateKeyNIDs[nid]; ok {
				assert.Equal(t, v, userID)
			} else {
				t.Fatalf("unable to find %d in result set", nid)
			}
		}
	})
}
