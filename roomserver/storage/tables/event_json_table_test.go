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

		tests := []struct {
			name      string
			args      []types.EventNID
			wantCount int
		}{
			{
				name:      "select subset of existing NIDs",
				args:      []types.EventNID{1, 2, 3, 4, 5},
				wantCount: 5,
			},
			{
				name:      "select subset of existing/non-existing NIDs",
				args:      []types.EventNID{1, 2, 12, 50},
				wantCount: 2,
			},
			{
				name:      "select single existing NID",
				args:      []types.EventNID{1},
				wantCount: 1,
			},
			{
				name:      "select single non-existing NID",
				args:      []types.EventNID{13},
				wantCount: 0,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				// select a subset of the data
				values, err := tab.BulkSelectEventJSON(context.Background(), nil, tc.args)
				assert.NoError(t, err)
				assert.Equal(t, tc.wantCount, len(values))
				for i, v := range values {
					assert.Equal(t, v.EventNID, types.EventNID(i+1))
					assert.Equal(t, []byte(fmt.Sprintf(`{"value":%d"}`, i+1)), v.EventJSON)
				}
			})
		}
	})
}
