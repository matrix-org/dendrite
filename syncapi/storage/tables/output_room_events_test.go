package tables_test

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage/postgres"
	"github.com/matrix-org/dendrite/syncapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
)

func newOutputRoomEventsTable(t *testing.T, dbType test.DBType) (tables.Events, *sql.DB, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	})
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}

	var tab tables.Events
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresEventsTable(db)
	case test.DBTypeSQLite:
		var stream sqlite3.StreamIDStatements
		if err = stream.Prepare(db); err != nil {
			t.Fatalf("failed to prepare stream stmts: %s", err)
		}
		tab, err = sqlite3.NewSqliteEventsTable(db, &stream)
	}
	if err != nil {
		t.Fatalf("failed to make new table: %s", err)
	}
	return tab, db, close
}

func TestOutputRoomEventsTable(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser()
	room := test.NewRoom(t, alice)
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, db, close := newOutputRoomEventsTable(t, dbType)
		defer close()
		events := room.Events()
		err := sqlutil.WithTransaction(db, func(txn *sql.Tx) error {
			for _, ev := range events {
				_, err := tab.InsertEvent(ctx, txn, ev, nil, nil, nil, false)
				if err != nil {
					return fmt.Errorf("failed to InsertEvent: %s", err)
				}
			}
			// order = 2,0,3,1
			wantEventIDs := []string{
				events[2].EventID(), events[0].EventID(), events[3].EventID(), events[1].EventID(),
			}
			gotEvents, err := tab.SelectEvents(ctx, txn, wantEventIDs, nil, true)
			if err != nil {
				return fmt.Errorf("failed to SelectEvents: %s", err)
			}
			gotEventIDs := make([]string, len(gotEvents))
			for i := range gotEvents {
				gotEventIDs[i] = gotEvents[i].EventID()
			}
			if !reflect.DeepEqual(gotEventIDs, wantEventIDs) {
				return fmt.Errorf("SelectEvents\ngot  %v\n want %v", gotEventIDs, wantEventIDs)
			}

			// Test that contains_url is correctly populated
			urlEv := room.CreateEvent(t, alice, "m.text", map[string]interface{}{
				"body": "test.txt",
				"url":  "mxc://test.txt",
			})
			if _, err = tab.InsertEvent(ctx, txn, urlEv, nil, nil, nil, false); err != nil {
				return fmt.Errorf("failed to InsertEvent: %s", err)
			}
			wantEventID := []string{urlEv.EventID()}
			t := true
			gotEvents, err = tab.SelectEvents(ctx, txn, wantEventID, &gomatrixserverlib.RoomEventFilter{Limit: 1, ContainsURL: &t}, true)
			if err != nil {
				return fmt.Errorf("failed to SelectEvents: %s", err)
			}
			gotEventIDs = make([]string, len(gotEvents))
			for i := range gotEvents {
				gotEventIDs[i] = gotEvents[i].EventID()
			}
			if !reflect.DeepEqual(gotEventIDs, wantEventID) {
				return fmt.Errorf("SelectEvents\ngot  %v\n want %v", gotEventIDs, wantEventID)
			}

			return nil
		})
		if err != nil {
			t.Fatalf("err: %s", err)
		}
	})
}
