package tables_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
)

func mustCreateEventJSONTable(t *testing.T, dbType test.DBType) (tables.EventJSON, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}
	var tab tables.EventJSON
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateEventJSONTable(db)
		if err != nil {
			t.Fatalf("failed to create EventJSON table: %s", err)
		}
		tab, err = postgres.PrepareEventJSONTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateEventJSONTable(db)
		if err != nil {
			t.Fatalf("failed to create EventJSON table: %s", err)
		}
		tab, err = sqlite3.PrepareEventJSONTable(db)
	}
	if err != nil {
		t.Fatalf("failed to create table: %s", err)
	}

	return tab, close
}

func Test_EventJSONTable(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateEventJSONTable(t, dbType)
		defer close()
		// create some dummy data
		for i := 0; i < 10; i++ {
			if err := tab.InsertEventJSON(
				context.Background(), nil, types.EventNID(i),
				[]byte(fmt.Sprintf(`{"value":%d"}`, i)),
			); err != nil {
				t.Fatalf("unable to insert eventJSON: %s", err)
			}
		}
		// select a subset of the data
		values, err := tab.BulkSelectEventJSON(context.Background(), nil, []types.EventNID{1, 2, 3, 4, 5})
		if err != nil {
			t.Fatalf("unable to query eventJSON: %s", err)
		}
		if len(values) != 5 {
			t.Fatalf("expected 5 events, got %d", len(values))
		}
		for i, v := range values {
			if v.EventNID != types.EventNID(i+1) {
				t.Fatalf("expected eventNID %d, got %d", i+1, v.EventNID)
			}
			wantValue := []byte(fmt.Sprintf(`{"value":%d"}`, i+1))
			if !reflect.DeepEqual(wantValue, v.EventJSON) {
				t.Fatalf("expected JSON to be %s, got %s", string(wantValue), string(v.EventJSON))
			}
		}
	})
}
