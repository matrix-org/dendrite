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
	"github.com/matrix-org/gomatrixserverlib"
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
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateEventsTable(t, dbType)
		defer close()
		// create some dummy data
		eventIDs := make([]string, 0, len(room.Events()))
		wantStateAtEvent := make([]types.StateAtEvent, 0, len(room.Events()))
		wantEventReferences := make([]gomatrixserverlib.EventReference, 0, len(room.Events()))
		wantStateAtEventAndRefs := make([]types.StateAtEventAndReference, 0, len(room.Events()))
		for _, ev := range room.Events() {
			eventNID, snapNID, err := tab.InsertEvent(ctx, nil, 1, 1, 1, ev.EventID(), ev.EventReference().EventSHA256, nil, ev.Depth(), false)
			assert.NoError(t, err)
			gotEventNID, gotSnapNID, err := tab.SelectEvent(ctx, nil, ev.EventID())
			assert.NoError(t, err)
			assert.Equal(t, eventNID, gotEventNID)
			assert.Equal(t, snapNID, gotSnapNID)
			eventID, err := tab.SelectEventID(ctx, nil, eventNID)
			assert.NoError(t, err)
			assert.Equal(t, eventID, ev.EventID())

			// The events shouldn't be sent to output yet
			sentToOutput, err := tab.SelectEventSentToOutput(ctx, nil, gotEventNID)
			assert.NoError(t, err)
			assert.False(t, sentToOutput)

			err = tab.UpdateEventSentToOutput(ctx, nil, gotEventNID)
			assert.NoError(t, err)

			// Now they should be sent to output
			sentToOutput, err = tab.SelectEventSentToOutput(ctx, nil, gotEventNID)
			assert.NoError(t, err)
			assert.True(t, sentToOutput)

			eventIDs = append(eventIDs, ev.EventID())
			wantEventReferences = append(wantEventReferences, ev.EventReference())

			// Set the stateSnapshot to 2 for some events to verify they are returned later
			stateSnapshot := 0
			if eventNID < 3 {
				stateSnapshot = 2
				err = tab.UpdateEventState(ctx, nil, eventNID, 2)
				assert.NoError(t, err)
			}
			stateAtEvent := types.StateAtEvent{
				BeforeStateSnapshotNID: types.StateSnapshotNID(stateSnapshot),
				IsRejected:             false,
				StateEntry: types.StateEntry{
					EventNID: eventNID,
					StateKeyTuple: types.StateKeyTuple{
						EventTypeNID:     1,
						EventStateKeyNID: 1,
					},
				},
			}
			wantStateAtEvent = append(wantStateAtEvent, stateAtEvent)
			wantStateAtEventAndRefs = append(wantStateAtEventAndRefs, types.StateAtEventAndReference{
				StateAtEvent:   stateAtEvent,
				EventReference: ev.EventReference(),
			})
		}

		stateEvents, err := tab.BulkSelectStateEventByID(ctx, nil, eventIDs, false)
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

		roomNIDs, err := tab.SelectRoomNIDsForEventNIDs(ctx, nil, nids)
		assert.NoError(t, err)
		// We only inserted one room, so the RoomNID should be the same for all evendNIDs
		for _, roomNID := range roomNIDs {
			assert.Equal(t, types.RoomNID(1), roomNID)
		}

		stateAtEvent, err := tab.BulkSelectStateAtEventByID(ctx, nil, eventIDs)
		assert.NoError(t, err)
		assert.Equal(t, len(eventIDs), len(stateAtEvent))

		assert.ElementsMatch(t, wantStateAtEvent, stateAtEvent)

		evendNIDMap, err := tab.BulkSelectEventID(ctx, nil, nids)
		assert.NoError(t, err)
		t.Logf("%+v", evendNIDMap)
		assert.Equal(t, len(evendNIDMap), len(nids))

		nidMap, err := tab.BulkSelectEventNID(ctx, nil, eventIDs)
		assert.NoError(t, err)
		// check that we got all expected eventNIDs
		for _, eventID := range eventIDs {
			_, ok := nidMap[eventID]
			assert.True(t, ok)
		}

		references, err := tab.BulkSelectEventReference(ctx, nil, nids)
		assert.NoError(t, err)
		assert.Equal(t, wantEventReferences, references)

		stateAndRefs, err := tab.BulkSelectStateAtEventAndReference(ctx, nil, nids)
		assert.NoError(t, err)
		assert.Equal(t, wantStateAtEventAndRefs, stateAndRefs)

		// check we get the expected event depth
		maxDepth, err := tab.SelectMaxEventDepth(ctx, nil, nids)
		assert.NoError(t, err)
		assert.Equal(t, int64(len(room.Events())+1), maxDepth)
	})
}
