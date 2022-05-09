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

func mustCreateEventJSONTable(t *testing.T, dbType test.DBType) (tables.EventJSON, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.EventJSON
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateEventJSONTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareEventJSONTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateEventJSONTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareEventJSONTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func Test_EventJSONTable(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateEventJSONTable(t, dbType)
		defer close()
		// create some dummy data
		for i := 0; i < 10; i++ {
			err := tab.InsertEventJSON(
				context.Background(), nil, types.EventNID(i),
				[]byte(fmt.Sprintf(`{"value":%d"}`, i)),
			)
			assert.NoError(t, err)
		}
		// select a subset of the data
		values, err := tab.BulkSelectEventJSON(context.Background(), nil, []types.EventNID{1, 2, 3, 4, 5})
		assert.NoError(t, err)
		assert.Equal(t, 5, len(values))
		for i, v := range values {
			assert.Equal(t, v.EventNID, types.EventNID(i+1))
			assert.Equal(t, []byte(fmt.Sprintf(`{"value":%d"}`, i+1)), v.EventJSON)
		}
	})
}
