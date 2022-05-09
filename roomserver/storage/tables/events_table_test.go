package tables_test

import (
	"context"
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

func mustCreateEventsTable(t *testing.T, dbType test.DBType) (tables.Events, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.Events
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateEventsTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareEventsTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateEventsTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareEventsTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func Test_EventsTable(t *testing.T) {
	alice := test.NewUser()
	room := test.NewRoom(t, alice)
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateEventsTable(t, dbType)
		defer close()
		// create some dummy data
		eventIDs := make([]string, 0, len(room.Events()))
		wantStateAtEvent := make([]types.StateAtEvent, 0, len(room.Events()))
		for _, ev := range room.Events() {
			eventIDs = append(eventIDs, ev.EventID())

			eventNID, snapNID, err := tab.InsertEvent(ctx, nil, 1, 1, 1, ev.EventID(), []byte(""), nil, 0, false)
			assert.NoError(t, err)
			gotEventNID, gotSnapNID, err := tab.SelectEvent(ctx, nil, ev.EventID())
			assert.NoError(t, err)
			assert.Equal(t, eventNID, gotEventNID)
			assert.Equal(t, snapNID, gotSnapNID)
			eventID, err := tab.SelectEventID(ctx, nil, eventNID)
			assert.NoError(t, err)
			assert.Equal(t, eventID, ev.EventID())

			wantStateAtEvent = append(wantStateAtEvent, types.StateAtEvent{
				Overwrite:              false,
				BeforeStateSnapshotNID: 0,
				IsRejected:             false,
				StateEntry: types.StateEntry{
					EventNID: eventNID,
					StateKeyTuple: types.StateKeyTuple{
						EventTypeNID:     1,
						EventStateKeyNID: 1,
					},
				},
			})
		}

		stateEvents, err := tab.BulkSelectStateEventByID(ctx, nil, eventIDs)
		assert.NoError(t, err)
		assert.Equal(t, len(stateEvents), len(eventIDs))
		nids := make([]types.EventNID, 0, len(stateEvents))
		for _, ev := range stateEvents {
			nids = append(nids, ev.EventNID)
		}
		stateEvents2, err := tab.BulkSelectStateEventByNID(ctx, nil, nids, nil)
		assert.NoError(t, err)
		// somehow SQLite doesn't return the values ordered as requested by the query
		assert.ElementsMatch(t, stateEvents, stateEvents2)

		stateAtEvent, err := tab.BulkSelectStateAtEventByID(ctx, nil, eventIDs)
		assert.NoError(t, err)
		assert.Equal(t, len(eventIDs), len(stateAtEvent))

		assert.ElementsMatch(t, wantStateAtEvent, stateAtEvent)

		evendNIDMap, err := tab.BulkSelectEventID(ctx, nil, nids)
		assert.NoError(t, err)
		t.Logf("%+v", evendNIDMap)
		assert.Equal(t, len(evendNIDMap), len(nids))
	})
}
