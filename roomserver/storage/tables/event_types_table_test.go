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
)

func mustCreateEventTypesTable(t *testing.T, dbType test.DBType) (tables.EventTypes, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}
	var tab tables.EventTypes
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateEventTypesTable(db)
		if err != nil {
			t.Fatalf("failed to create EventJSON table: %s", err)
		}
		tab, err = postgres.PrepareEventTypesTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateEventTypesTable(db)
		if err != nil {
			t.Fatalf("failed to create EventJSON table: %s", err)
		}
		tab, err = sqlite3.PrepareEventTypesTable(db)
	}
	if err != nil {
		t.Fatalf("failed to create table: %s", err)
	}

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
			if eventTypeNID, err = tab.InsertEventTypeNID(
				ctx, nil, eventType,
			); err != nil {
				t.Fatalf("unable to insert eventJSON: %s", err)
			}
			eventTypeMap[eventType] = eventTypeNID
			gotEventTypeNID, err = tab.SelectEventTypeNID(ctx, nil, eventType)
			if err != nil {
				t.Fatalf("failed to get EventTypeNID: %s", err)
			}
			if eventTypeNID != gotEventTypeNID {
				t.Fatalf("expected eventTypeNID %d, but got %d", eventTypeNID, gotEventTypeNID)
			}
		}
		eventTypeNIDs, err := tab.BulkSelectEventTypeNID(ctx, nil, []string{"dummyEventType0", "dummyEventType3"})
		if err != nil {
			t.Fatalf("failed to get EventStateKeyNIDs: %s", err)
		}
		// verify that BulkSelectEventTypeNID and InsertEventTypeNID return the same values
		for eventType, nid := range eventTypeNIDs {
			if v, ok := eventTypeMap[eventType]; ok {
				if v != nid {
					t.Fatalf("EventTypeNID does not match: %d != %d", nid, v)
				}
			} else {
				t.Fatalf("unable to find %d in result set", nid)
			}
		}
	})
}
