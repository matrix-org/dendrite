package tables_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/storage/postgres"
	"github.com/element-hq/dendrite/roomserver/storage/sqlite3"
	"github.com/element-hq/dendrite/roomserver/storage/tables"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/test"
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
		wantStateAtEventAndRefs := make([]types.StateAtEventAndReference, 0, len(room.Events()))
		for _, ev := range room.Events() {
			eventNID, snapNID, err := tab.InsertEvent(ctx, nil, 1, 1, 1, ev.EventID(), nil, ev.Depth(), false)
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
				StateAtEvent: stateAtEvent,
				EventID:      ev.EventID(),
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

		stateAndRefs, err := tab.BulkSelectStateAtEventAndReference(ctx, nil, nids)
		assert.NoError(t, err)
		assert.Equal(t, wantStateAtEventAndRefs, stateAndRefs)

		// check we get the expected event depth
		maxDepth, err := tab.SelectMaxEventDepth(ctx, nil, nids)
		assert.NoError(t, err)
		assert.Equal(t, int64(len(room.Events())+1), maxDepth)
	})
}

func TestRoomsWithACL(t *testing.T) {

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		eventStateKeys, closeEventStateKeys := mustCreateEventTypesTable(t, dbType)
		defer closeEventStateKeys()

		eventsTable, closeEventsTable := mustCreateEventsTable(t, dbType)
		defer closeEventsTable()

		ctx := context.Background()

		// insert the m.room.server_acl event type
		eventTypeNID, err := eventStateKeys.InsertEventTypeNID(ctx, nil, "m.room.server_acl")
		assert.Nil(t, err)

		// Create ACL'd rooms
		var wantRoomNIDs []types.RoomNID
		for i := 0; i < 10; i++ {
			_, _, err = eventsTable.InsertEvent(ctx, nil, types.RoomNID(i), eventTypeNID, types.EmptyStateKeyNID, fmt.Sprintf("$1337+%d", i), nil, 0, false)
			assert.Nil(t, err)
			wantRoomNIDs = append(wantRoomNIDs, types.RoomNID(i))
		}

		// Create non-ACL'd rooms (eventTypeNID+1)
		for i := 10; i < 20; i++ {
			_, _, err = eventsTable.InsertEvent(ctx, nil, types.RoomNID(i), eventTypeNID+1, types.EmptyStateKeyNID, fmt.Sprintf("$1337+%d", i), nil, 0, false)
			assert.Nil(t, err)
		}

		gotRoomNIDs, err := eventsTable.SelectRoomsWithEventTypeNID(ctx, nil, eventTypeNID)
		assert.Nil(t, err)
		assert.Equal(t, wantRoomNIDs, gotRoomNIDs)
	})
}
