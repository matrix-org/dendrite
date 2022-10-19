package tables_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage/postgres"
	"github.com/matrix-org/dendrite/syncapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/test"
)

func newCurrentRoomStateTable(t *testing.T, dbType test.DBType) (tables.CurrentRoomState, *sql.DB, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}

	var tab tables.CurrentRoomState
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresCurrentRoomStateTable(db)
	case test.DBTypeSQLite:
		var stream sqlite3.StreamIDStatements
		if err = stream.Prepare(db); err != nil {
			t.Fatalf("failed to prepare stream stmts: %s", err)
		}
		tab, err = sqlite3.NewSqliteCurrentRoomStateTable(db, &stream)
	}
	if err != nil {
		t.Fatalf("failed to make new table: %s", err)
	}
	return tab, db, close
}

func TestCurrentRoomStateTable(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, db, close := newCurrentRoomStateTable(t, dbType)
		defer close()
		events := room.CurrentState()
		err := sqlutil.WithTransaction(db, func(txn *sql.Tx) error {
			for i, ev := range events {
				err := tab.UpsertRoomState(ctx, txn, ev, nil, types.StreamPosition(i))
				if err != nil {
					return fmt.Errorf("failed to UpsertRoomState: %w", err)
				}
			}
			wantEventIDs := []string{
				events[0].EventID(), events[1].EventID(), events[2].EventID(), events[3].EventID(),
			}
			gotEvents, err := tab.SelectEventsWithEventIDs(ctx, txn, wantEventIDs)
			if err != nil {
				return fmt.Errorf("failed to SelectEventsWithEventIDs: %w", err)
			}
			if len(gotEvents) != len(wantEventIDs) {
				return fmt.Errorf("SelectEventsWithEventIDs\ngot %d, want %d results", len(gotEvents), len(wantEventIDs))
			}
			gotEventIDs := make(map[string]struct{}, len(gotEvents))
			for _, event := range gotEvents {
				if event.ExcludeFromSync {
					return fmt.Errorf("SelectEventsWithEventIDs ExcludeFromSync should be false for current room state event %+v", event)
				}
				gotEventIDs[event.EventID()] = struct{}{}
			}
			for _, id := range wantEventIDs {
				if _, ok := gotEventIDs[id]; !ok {
					return fmt.Errorf("SelectEventsWithEventIDs\nexpected id %q not returned", id)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
	})
}
