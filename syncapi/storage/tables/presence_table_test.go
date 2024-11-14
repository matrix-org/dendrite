package tables_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/syncapi/storage/postgres"
	"github.com/element-hq/dendrite/syncapi/storage/sqlite3"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/element-hq/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

func mustPresenceTable(t *testing.T, dbType test.DBType) (tables.Presence, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}

	var tab tables.Presence
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresPresenceTable(db)
	case test.DBTypeSQLite:
		var stream sqlite3.StreamIDStatements
		if err = stream.Prepare(db); err != nil {
			t.Fatalf("failed to prepare stream stmts: %s", err)
		}
		tab, err = sqlite3.NewSqlitePresenceTable(db, &stream)
	}
	if err != nil {
		t.Fatalf("failed to make new table: %s", err)
	}
	return tab, close
}

func TestPresence(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	ctx := context.Background()

	statusMsg := "Hello World!"
	timestamp := spec.AsTimestamp(time.Now())

	var txn *sql.Tx
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, closeDB := mustPresenceTable(t, dbType)
		defer closeDB()

		// Insert some presences
		pos, err := tab.UpsertPresence(ctx, txn, alice.ID, &statusMsg, types.PresenceOnline, timestamp, false)
		if err != nil {
			t.Error(err)
		}
		wantPos := types.StreamPosition(1)
		if pos != wantPos {
			t.Errorf("expected pos to be %d, got %d", wantPos, pos)
		}
		pos, err = tab.UpsertPresence(ctx, txn, bob.ID, &statusMsg, types.PresenceOnline, timestamp, false)
		if err != nil {
			t.Error(err)
		}
		wantPos = 2
		if pos != wantPos {
			t.Errorf("expected pos to be %d, got %d", wantPos, pos)
		}

		// verify the expected max presence ID
		maxPos, err := tab.GetMaxPresenceID(ctx, txn)
		if err != nil {
			t.Error(err)
		}
		if maxPos != wantPos {
			t.Errorf("expected max pos to be %d, got %d", wantPos, maxPos)
		}

		// This should increment the position
		pos, err = tab.UpsertPresence(ctx, txn, bob.ID, &statusMsg, types.PresenceOnline, timestamp, true)
		if err != nil {
			t.Error(err)
		}
		wantPos = pos
		if wantPos <= maxPos {
			t.Errorf("expected pos to be %d incremented, got %d", wantPos, pos)
		}

		// This should return only Bobs status
		presences, err := tab.GetPresenceAfter(ctx, txn, maxPos, synctypes.EventFilter{Limit: 10})
		if err != nil {
			t.Error(err)
		}

		if c := len(presences); c > 1 {
			t.Errorf("expected only one presence, got %d", c)
		}

		// Validate the response
		wantPresence := &types.PresenceInternal{
			UserID:       bob.ID,
			Presence:     types.PresenceOnline,
			StreamPos:    wantPos,
			LastActiveTS: timestamp,
			ClientFields: types.PresenceClientResponse{
				LastActiveAgo: 0,
				Presence:      types.PresenceOnline.String(),
				StatusMsg:     &statusMsg,
			},
		}
		if !reflect.DeepEqual(wantPresence, presences[bob.ID]) {
			t.Errorf("unexpected presence result:\n%+v, want\n%+v", presences[bob.ID], wantPresence)
		}

		// Try getting presences for existing and non-existing users
		getUsers := []string{alice.ID, bob.ID, "@doesntexist:test"}
		presencesForUsers, err := tab.GetPresenceForUsers(ctx, nil, getUsers)
		if err != nil {
			t.Error(err)
		}

		if len(presencesForUsers) >= len(getUsers) {
			t.Errorf("expected less presences, but they are the same/more as requested: %d >= %d", len(presencesForUsers), len(getUsers))
		}
	})

}
