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
	"github.com/stretchr/testify/assert"
)

func newTopologyTable(t *testing.T, dbType test.DBType) (tables.Topology, *sql.DB, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}

	var tab tables.Topology
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresTopologyTable(db)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSqliteTopologyTable(db)
	}
	if err != nil {
		t.Fatalf("failed to make new table: %s", err)
	}
	return tab, db, close
}

func TestTopologyTable(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, db, close := newTopologyTable(t, dbType)
		defer close()
		events := room.Events()
		err := sqlutil.WithTransaction(db, func(txn *sql.Tx) error {
			var highestPos types.StreamPosition
			for i, ev := range events {
				topoPos, err := tab.InsertEventInTopology(ctx, txn, ev, types.StreamPosition(i))
				if err != nil {
					return fmt.Errorf("failed to InsertEventInTopology: %s", err)
				}
				// topo pos = depth, depth starts at 1, hence 1+i
				if topoPos != types.StreamPosition(1+i) {
					return fmt.Errorf("got topo pos %d want %d", topoPos, 1+i)
				}
				highestPos = topoPos + 1
			}
			// check ordering works without limit
			eventIDs, start, end, err := tab.SelectEventIDsInRange(ctx, txn, room.ID, 0, highestPos, highestPos, 100, true)
			assert.NoError(t, err, "failed to SelectEventIDsInRange")
			test.AssertEventIDsEqual(t, eventIDs, events[:])
			assert.Equal(t, types.TopologyToken{Depth: 1, PDUPosition: 0}, start)
			assert.Equal(t, types.TopologyToken{Depth: 5, PDUPosition: 4}, end)

			eventIDs, start, end, err = tab.SelectEventIDsInRange(ctx, txn, room.ID, 0, highestPos, highestPos, 100, false)
			assert.NoError(t, err, "failed to SelectEventIDsInRange")
			test.AssertEventIDsEqual(t, eventIDs, test.Reversed(events[:]))
			assert.Equal(t, types.TopologyToken{Depth: 5, PDUPosition: 4}, start)
			assert.Equal(t, types.TopologyToken{Depth: 1, PDUPosition: 0}, end)

			// check ordering works with limit
			eventIDs, start, end, err = tab.SelectEventIDsInRange(ctx, txn, room.ID, 0, highestPos, highestPos, 3, true)
			assert.NoError(t, err, "failed to SelectEventIDsInRange")
			test.AssertEventIDsEqual(t, eventIDs, events[:3])
			assert.Equal(t, types.TopologyToken{Depth: 1, PDUPosition: 0}, start)
			assert.Equal(t, types.TopologyToken{Depth: 3, PDUPosition: 2}, end)

			eventIDs, start, end, err = tab.SelectEventIDsInRange(ctx, txn, room.ID, 0, highestPos, highestPos, 3, false)
			assert.NoError(t, err, "failed to SelectEventIDsInRange")
			test.AssertEventIDsEqual(t, eventIDs, test.Reversed(events[len(events)-3:]))
			assert.Equal(t, types.TopologyToken{Depth: 5, PDUPosition: 4}, start)
			assert.Equal(t, types.TopologyToken{Depth: 3, PDUPosition: 2}, end)

			// Check that we return no values for invalid rooms
			eventIDs, start, end, err = tab.SelectEventIDsInRange(ctx, txn, "!doesnotexist:localhost", 0, highestPos, highestPos, 10, false)
			assert.NoError(t, err, "failed to SelectEventIDsInRange")
			assert.Equal(t, 0, len(eventIDs))
			assert.Equal(t, types.TopologyToken{}, start)
			assert.Equal(t, types.TopologyToken{}, end)
			return nil
		})
		if err != nil {
			t.Fatalf("err: %s", err)
		}
	})
}
