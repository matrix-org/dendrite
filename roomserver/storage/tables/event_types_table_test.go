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

func mustCreateEventTypesTable(t *testing.T, dbType test.DBType) (tables.EventTypes, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.EventTypes
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateEventTypesTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareEventTypesTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateEventTypesTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareEventTypesTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func Test_EventTypesTable(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateEventTypesTable(t, dbType)
		defer close()
		ctx := context.Background()
		var eventTypeNID, gotEventTypeNID types.EventTypeNID
		var err error
		// create some dummy data
		eventTypeMap := make(map[string]types.EventTypeNID)
		for i := 0; i < 10; i++ {
			eventType := fmt.Sprintf("dummyEventType%d", i)
			eventTypeNID, err = tab.InsertEventTypeNID(ctx, nil, eventType)
			assert.NoError(t, err)
			eventTypeMap[eventType] = eventTypeNID
			gotEventTypeNID, err = tab.SelectEventTypeNID(ctx, nil, eventType)
			assert.NoError(t, err)
			assert.Equal(t, eventTypeNID, gotEventTypeNID)
		}
		// This should fail, since the dummyEventType0 already exists
		eventType := fmt.Sprintf("dummyEventType%d", 0)
		_, err = tab.InsertEventTypeNID(ctx, nil, eventType)
		assert.Error(t, err)

		// This should return an error, as this eventType does not exist
		_, err = tab.SelectEventTypeNID(ctx, nil, "dummyEventType13")
		assert.Error(t, err)

		eventTypeNIDs, err := tab.BulkSelectEventTypeNID(ctx, nil, []string{"dummyEventType0", "dummyEventType3"})
		assert.NoError(t, err)
		// verify that BulkSelectEventTypeNID and InsertEventTypeNID return the same values
		for eventType, nid := range eventTypeNIDs {
			if v, ok := eventTypeMap[eventType]; ok {
				assert.Equal(t, v, nid)
			} else {
				t.Fatalf("unable to find %d in result set", nid)
			}
		}
	})
}
