package tables_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage/postgres"
	"github.com/matrix-org/dendrite/syncapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/test"
)

func newRelationsTable(t *testing.T, dbType test.DBType) (tables.Relations, *sql.DB, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}

	var tab tables.Relations
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresRelationsTable(db)
	case test.DBTypeSQLite:
		var stream sqlite3.StreamIDStatements
		if err = stream.Prepare(db); err != nil {
			t.Fatalf("failed to prepare stream stmts: %s", err)
		}
		tab, err = sqlite3.NewSqliteRelationsTable(db, &stream)
	}
	if err != nil {
		t.Fatalf("failed to make new table: %s", err)
	}
	return tab, db, close
}

func compareRelationsToExpected(t *testing.T, tab tables.Relations, r types.Range, expected []types.RelationEntry) {
	ctx := context.Background()
	relations, _, err := tab.SelectRelationsInRange(ctx, nil, roomID, "a", "", "", r, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(relations[relType]) != len(expected) {
		t.Fatalf("incorrect number of values returned for range %v (got %d, want %d)", r, len(relations[relType]), len(expected))
	}
	for i := 0; i < len(relations[relType]); i++ {
		got := relations[relType][i]
		want := expected[i]
		if got != want {
			t.Fatalf("range %v position %d should have been %q but got %q", r, i, got, want)
		}
	}
}

const roomID = "!roomid:server"
const childType = "m.room.something"
const relType = "m.reaction"

func TestRelationsTable(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, _, close := newRelationsTable(t, dbType)
		defer close()

		// Insert some relations
		for _, child := range []string{"b", "c", "d"} {
			if err := tab.InsertRelation(ctx, nil, roomID, "a", child, childType, relType); err != nil {
				t.Fatal(err)
			}
		}

		// Check the max position, we've inserted three things so it
		// should be 3
		if max, err := tab.SelectMaxRelationID(ctx, nil); err != nil {
			t.Fatal(err)
		} else if max != 3 {
			t.Fatalf("max position should have been 3 but got %d", max)
		}

		// Query some ranges for "a"
		for r, expected := range map[types.Range][]types.RelationEntry{
			{From: 0, To: 10, Backwards: false}: {
				{Position: 1, EventID: "b"},
				{Position: 2, EventID: "c"},
				{Position: 3, EventID: "d"},
			},
			{From: 1, To: 2, Backwards: false}: {
				{Position: 2, EventID: "c"},
			},
			{From: 1, To: 3, Backwards: false}: {
				{Position: 2, EventID: "c"},
				{Position: 3, EventID: "d"},
			},
			{From: 10, To: 0, Backwards: true}: {
				{Position: 3, EventID: "d"},
				{Position: 2, EventID: "c"},
				{Position: 1, EventID: "b"},
			},
			{From: 3, To: 1, Backwards: true}: {
				{Position: 2, EventID: "c"},
				{Position: 1, EventID: "b"},
			},
		} {
			compareRelationsToExpected(t, tab, r, expected)
		}

		// Now delete one of the relations
		if err := tab.DeleteRelation(ctx, nil, roomID, "c"); err != nil {
			t.Fatal(err)
		}

		// Query some more ranges for "a"
		for r, expected := range map[types.Range][]types.RelationEntry{
			{From: 0, To: 10, Backwards: false}: {
				{Position: 1, EventID: "b"},
				{Position: 3, EventID: "d"},
			},
			{From: 1, To: 2, Backwards: false}: {},
			{From: 1, To: 3, Backwards: false}: {
				{Position: 3, EventID: "d"},
			},
			{From: 10, To: 0, Backwards: true}: {
				{Position: 3, EventID: "d"},
				{Position: 1, EventID: "b"},
			},
			{From: 3, To: 1, Backwards: true}: {
				{Position: 1, EventID: "b"},
			},
		} {
			compareRelationsToExpected(t, tab, r, expected)
		}

		// Insert some new relations
		for _, child := range []string{"e", "f", "g", "h"} {
			if err := tab.InsertRelation(ctx, nil, roomID, "a", child, childType, relType); err != nil {
				t.Fatal(err)
			}
		}

		// Check the max position, we've inserted four things so it
		// should now be 7
		if max, err := tab.SelectMaxRelationID(ctx, nil); err != nil {
			t.Fatal(err)
		} else if max != 7 {
			t.Fatalf("max position should have been 3 but got %d", max)
		}

		// Query last set of ranges for "a"
		for r, expected := range map[types.Range][]types.RelationEntry{
			{From: 0, To: 10, Backwards: false}: {
				{Position: 1, EventID: "b"},
				{Position: 3, EventID: "d"},
				{Position: 4, EventID: "e"},
				{Position: 5, EventID: "f"},
				{Position: 6, EventID: "g"},
				{Position: 7, EventID: "h"},
			},
			{From: 1, To: 2, Backwards: false}: {},
			{From: 1, To: 3, Backwards: false}: {
				{Position: 3, EventID: "d"},
			},
			{From: 10, To: 0, Backwards: true}: {
				{Position: 7, EventID: "h"},
				{Position: 6, EventID: "g"},
				{Position: 5, EventID: "f"},
				{Position: 4, EventID: "e"},
				{Position: 3, EventID: "d"},
				{Position: 1, EventID: "b"},
			},
			{From: 6, To: 3, Backwards: true}: {
				{Position: 5, EventID: "f"},
				{Position: 4, EventID: "e"},
				{Position: 3, EventID: "d"},
			},
		} {
			compareRelationsToExpected(t, tab, r, expected)
		}
	})
}
