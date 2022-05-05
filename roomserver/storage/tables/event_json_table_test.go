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

	var (
		tab tables.EventJSON
	)
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateEventJSONTable(db)
		if err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
		tab, err = postgres.PrepareEventJSONTable(db)
		if err != nil {
			t.Fatalf("failed to prepare statements: %v", err)
		}
	case test.DBTypeSQLite:
		err = sqlite3.CreateEventJSONTable(db)
		if err != nil {
			t.Fatalf("failed to create table: %v", err)
		}
		tab, err = sqlite3.PrepareEventJSONTable(db)
		if err != nil {
			t.Fatalf("failed to prepare statements: %v", err)
		}
	}
	if err != nil {
		t.Fatalf("failed to make new table: %s", err)
	}
	return tab, close
}

func TestEventJSONTable(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateEventJSONTable(t, dbType)
		defer close()
		// insert some dummy data
		for i := 0; i < 10; i++ {
			err := tab.InsertEventJSON(ctx, nil, types.EventNID(i), []byte(fmt.Sprintf(`{"value": %d }`, i)))
			if err != nil {
				t.Fatalf("failed to insert event json: %v", err)
			}
		}
		// and fetch them again
		eventJSONPair, err := tab.BulkSelectEventJSON(ctx, nil, types.EventNIDs{1, 2, 3, 4, 5})
		if err != nil {
			t.Fatalf("failed to get event json: %v", err)
		}
		if len(eventJSONPair) != 5 {
			t.Fatalf("expected 5 events, got %d", len(eventJSONPair))
		}
		for _, x := range eventJSONPair {
			want := []byte(fmt.Sprintf(`{"value": %d }`, x.EventNID))
			if !reflect.DeepEqual(x.EventJSON, want) {
				t.Fatalf("unexpected eventJSON %v, want %v", string(x.EventJSON), string(want))
			}
		}
	})
}
