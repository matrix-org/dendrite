package storage_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	rstypes "github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/synctypes"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

var ctx = context.Background()

func MustCreateDatabase(t *testing.T, dbType test.DBType) (storage.Database, func()) {
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})
	db, err := storage.NewSyncServerDatasource(context.Background(), cm, &config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	})
	if err != nil {
		t.Fatalf("NewSyncServerDatasource returned %s", err)
	}
	return db, close
}

func MustWriteEvents(t *testing.T, db storage.Database, events []*rstypes.HeaderedEvent) (positions []types.StreamPosition) {
	for _, ev := range events {
		var addStateEvents []*rstypes.HeaderedEvent
		var addStateEventIDs []string
		var removeStateEventIDs []string
		if ev.StateKey() != nil {
			ev.StateKeyResolved = ev.StateKey()
			addStateEvents = append(addStateEvents, ev)
			addStateEventIDs = append(addStateEventIDs, ev.EventID())
		}
		pos, err := db.WriteEvent(ctx, ev, addStateEvents, addStateEventIDs, removeStateEventIDs, nil, false, gomatrixserverlib.HistoryVisibilityShared)
		if err != nil {
			t.Fatalf("WriteEvent failed: %s", err)
		}
		t.Logf("Event ID %s spos=%v depth=%v", ev.EventID(), pos, ev.Depth())
		positions = append(positions, pos)
	}
	return
}

func TestWriteEvents(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		alice := test.NewUser(t)
		r := test.NewRoom(t, alice)
		db, close := MustCreateDatabase(t, dbType)
		defer close()
		MustWriteEvents(t, db, r.Events())
	})
}

func WithSnapshot(t *testing.T, db storage.Database, f func(snapshot storage.DatabaseTransaction)) {
	snapshot, err := db.NewDatabaseSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f(snapshot)
	if err := snapshot.Rollback(); err != nil {
		t.Fatal(err)
	}
}

// These tests assert basic functionality of RecentEvents for PDUs
func TestRecentEventsPDU(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := MustCreateDatabase(t, dbType)
		defer close()
		alice := test.NewUser(t)
		// dummy room to make sure SQL queries are filtering on room ID
		MustWriteEvents(t, db, test.NewRoom(t, alice).Events())

		// actual test room
		r := test.NewRoom(t, alice)
		r.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": "hi"})
		events := r.Events()
		positions := MustWriteEvents(t, db, events)

		// dummy room to make sure SQL queries are filtering on room ID
		MustWriteEvents(t, db, test.NewRoom(t, alice).Events())

		var latest types.StreamPosition
		WithSnapshot(t, db, func(snapshot storage.DatabaseTransaction) {
			var err error
			if latest, err = snapshot.MaxStreamPositionForPDUs(ctx); err != nil {
				t.Fatal("failed to get MaxStreamPositionForPDUs: %w", err)
			}
		})

		testCases := []struct {
			Name         string
			From         types.StreamPosition
			To           types.StreamPosition
			Limit        int
			ReverseOrder bool
			WantEvents   []*rstypes.HeaderedEvent
			WantLimited  bool
		}{
			// The purpose of this test is to make sure that incremental syncs are including up to the latest events.
			// It's a basic sanity test that sync works. It creates a streaming position that is on the penultimate event.
			// It makes sure the response includes the final event.
			{
				Name:        "penultimate",
				From:        positions[len(positions)-2], // pretend we are at the penultimate event
				To:          latest,
				Limit:       100,
				WantEvents:  events[len(events)-1:],
				WantLimited: false,
			},
			// The purpose of this test is to check that limits can be applied and work.
			// This is critical for big rooms hence the test here.
			{
				Name:        "limited",
				From:        0,
				To:          latest,
				Limit:       1,
				WantEvents:  events[len(events)-1:],
				WantLimited: true,
			},
			// The purpose of this test is to check that we can return every event with a high
			// enough limit
			{
				Name:        "large limited",
				From:        0,
				To:          latest,
				Limit:       100,
				WantEvents:  events,
				WantLimited: false,
			},
			// The purpose of this test is to check that we can return events in reverse order
			{
				Name:         "reverse",
				From:         positions[len(positions)-3], // 2 events back
				To:           latest,
				Limit:        100,
				ReverseOrder: true,
				WantEvents:   test.Reversed(events[len(events)-2:]),
				WantLimited:  false,
			},
		}

		for i := range testCases {
			tc := testCases[i]
			t.Run(tc.Name, func(st *testing.T) {
				var filter synctypes.RoomEventFilter
				var gotEvents map[string]types.RecentEvents
				var limited bool
				filter.Limit = tc.Limit
				WithSnapshot(t, db, func(snapshot storage.DatabaseTransaction) {
					var err error
					gotEvents, err = snapshot.RecentEvents(ctx, []string{r.ID}, types.Range{
						From: tc.From,
						To:   tc.To,
					}, &filter, !tc.ReverseOrder, true)
					if err != nil {
						st.Fatalf("failed to do sync: %s", err)
					}
				})
				streamEvents := gotEvents[r.ID]
				limited = streamEvents.Limited
				if limited != tc.WantLimited {
					st.Errorf("got limited=%v want %v", limited, tc.WantLimited)
				}
				if len(streamEvents.Events) != len(tc.WantEvents) {
					st.Errorf("got %d events, want %d", len(gotEvents), len(tc.WantEvents))
				}

				for j := range streamEvents.Events {
					if !reflect.DeepEqual(streamEvents.Events[j].JSON(), tc.WantEvents[j].JSON()) {
						st.Errorf("event %d got %s want %s", j, string(streamEvents.Events[j].JSON()), string(tc.WantEvents[j].JSON()))
					}
				}
			})
		}
	})
}

// The purpose of this test is to ensure that backfill does indeed go backwards, using a topology token
func TestGetEventsInRangeWithTopologyToken(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := MustCreateDatabase(t, dbType)
		defer close()
		alice := test.NewUser(t)
		r := test.NewRoom(t, alice)
		for i := 0; i < 10; i++ {
			r.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": fmt.Sprintf("hi %d", i)})
		}
		events := r.Events()
		_ = MustWriteEvents(t, db, events)

		WithSnapshot(t, db, func(snapshot storage.DatabaseTransaction) {
			from := types.TopologyToken{Depth: math.MaxInt64, PDUPosition: math.MaxInt64}
			t.Logf("max topo pos = %+v", from)
			// head towards the beginning of time
			to := types.TopologyToken{}

			// backpaginate 5 messages starting at the latest position.
			filter := &synctypes.RoomEventFilter{Limit: 5}
			paginatedEvents, start, end, err := snapshot.GetEventsInTopologicalRange(ctx, &from, &to, r.ID, filter, true)
			if err != nil {
				t.Fatalf("GetEventsInTopologicalRange returned an error: %s", err)
			}
			gots := snapshot.StreamEventsToEvents(context.Background(), nil, paginatedEvents, nil)
			test.AssertEventsEqual(t, gots, test.Reversed(events[len(events)-5:]))
			assert.Equal(t, types.TopologyToken{Depth: 15, PDUPosition: 15}, start)
			assert.Equal(t, types.TopologyToken{Depth: 11, PDUPosition: 11}, end)
		})
	})
}

// The purpose of this test is to ensure that backfilling returns no start/end if a given filter removes
// all events.
func TestGetEventsInRangeWithTopologyTokenNoEventsForFilter(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := MustCreateDatabase(t, dbType)
		defer close()
		alice := test.NewUser(t)
		r := test.NewRoom(t, alice)
		for i := 0; i < 10; i++ {
			r.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": fmt.Sprintf("hi %d", i)})
		}
		events := r.Events()
		_ = MustWriteEvents(t, db, events)

		WithSnapshot(t, db, func(snapshot storage.DatabaseTransaction) {
			from := types.TopologyToken{Depth: math.MaxInt64, PDUPosition: math.MaxInt64}
			t.Logf("max topo pos = %+v", from)
			// head towards the beginning of time
			to := types.TopologyToken{}

			// backpaginate 20 messages starting at the latest position.
			notTypes := []string{spec.MRoomRedaction}
			senders := []string{alice.ID}
			filter := &synctypes.RoomEventFilter{Limit: 20, NotTypes: &notTypes, Senders: &senders}
			paginatedEvents, start, end, err := snapshot.GetEventsInTopologicalRange(ctx, &from, &to, r.ID, filter, true)
			assert.NoError(t, err)
			assert.Equal(t, 0, len(paginatedEvents))
			// Even if we didn't get anything back due to the filter, we should still have start/end
			assert.Equal(t, types.TopologyToken{Depth: 15, PDUPosition: 15}, start)
			assert.Equal(t, types.TopologyToken{Depth: 1, PDUPosition: 1}, end)
		})
	})
}

func TestStreamToTopologicalPosition(t *testing.T) {
	alice := test.NewUser(t)
	r := test.NewRoom(t, alice)

	testCases := []struct {
		name             string
		roomID           string
		streamPos        types.StreamPosition
		backwardOrdering bool
		wantToken        types.TopologyToken
	}{
		{
			name:             "forward ordering found streamPos returns found position",
			roomID:           r.ID,
			streamPos:        1,
			backwardOrdering: false,
			wantToken:        types.TopologyToken{Depth: 1, PDUPosition: 1},
		},
		{
			name:             "forward ordering not found streamPos returns max position",
			roomID:           r.ID,
			streamPos:        100,
			backwardOrdering: false,
			wantToken:        types.TopologyToken{Depth: math.MaxInt64, PDUPosition: math.MaxInt64},
		},
		{
			name:             "backward ordering found streamPos returns found position",
			roomID:           r.ID,
			streamPos:        1,
			backwardOrdering: true,
			wantToken:        types.TopologyToken{Depth: 1, PDUPosition: 1},
		},
		{
			name:             "backward ordering not found streamPos returns maxDepth with param pduPosition",
			roomID:           r.ID,
			streamPos:        100,
			backwardOrdering: true,
			wantToken:        types.TopologyToken{Depth: 5, PDUPosition: 100},
		},
		{
			name:             "backward non-existent room returns zero token",
			roomID:           "!doesnotexist:localhost",
			streamPos:        1,
			backwardOrdering: true,
			wantToken:        types.TopologyToken{Depth: 0, PDUPosition: 1},
		},
		{
			name:             "forward non-existent room returns max token",
			roomID:           "!doesnotexist:localhost",
			streamPos:        1,
			backwardOrdering: false,
			wantToken:        types.TopologyToken{Depth: math.MaxInt64, PDUPosition: math.MaxInt64},
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := MustCreateDatabase(t, dbType)
		defer close()

		txn, err := db.NewDatabaseTransaction(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer txn.Rollback()
		MustWriteEvents(t, db, r.Events())

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				token, err := txn.StreamToTopologicalPosition(ctx, tc.roomID, tc.streamPos, tc.backwardOrdering)
				if err != nil {
					t.Fatal(err)
				}
				if tc.wantToken != token {
					t.Fatalf("expected token %q, got %q", tc.wantToken, token)
				}
			})
		}

	})
}

/*
// The purpose of this test is to make sure that backpagination returns all events, even if some events have the same depth.
// For cases where events have the same depth, the streaming token should be used to tie break so events written via WriteEvent
// will appear FIRST when going backwards. This test creates a DAG like:
//                            .-----> Message ---.
//     Create -> Membership --------> Message -------> Message
//                            `-----> Message ---`
// depth  1          2                   3                 4
//
// With a total depth of 4. It tests that:
// - Backpagination over the whole fork should include all messages and not leave any out.
// - Backpagination from the middle of the fork should not return duplicates (things later than the token).
func TestGetEventsInRangeWithEventsSameDepth(t *testing.T) {
	t.Parallel()
	db := MustCreateDatabase(t)

	var events []*types.HeaderedEvent
	events = append(events, MustCreateEvent(t, testRoomID, nil, &gomatrixserverlib.EventBuilder{
		Content:  []byte(fmt.Sprintf(`{"room_version":"4","creator":"%s"}`, testUserIDA)),
		Type:     "m.room.create",
		StateKey: &emptyStateKey,
		Sender:   testUserIDA,
		Depth:    int64(len(events) + 1),
	}))
	events = append(events, MustCreateEvent(t, testRoomID, []*types.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
		Content:  []byte(`{"membership":"join"}`),
		Type:     "m.room.member",
		StateKey: &testUserIDA,
		Sender:   testUserIDA,
		Depth:    int64(len(events) + 1),
	}))
	// fork the dag into three, same prev_events and depth
	parent := []*types.HeaderedEvent{events[len(events)-1]}
	depth := int64(len(events) + 1)
	for i := 0; i < 3; i++ {
		events = append(events, MustCreateEvent(t, testRoomID, parent, &gomatrixserverlib.EventBuilder{
			Content: []byte(fmt.Sprintf(`{"body":"Message A %d"}`, i+1)),
			Type:    "m.room.message",
			Sender:  testUserIDA,
			Depth:   depth,
		}))
	}
	// merge the fork, prev_events are all 3 messages, depth is increased by 1.
	events = append(events, MustCreateEvent(t, testRoomID, events[len(events)-3:], &gomatrixserverlib.EventBuilder{
		Content: []byte(`{"body":"Message merge"}`),
		Type:    "m.room.message",
		Sender:  testUserIDA,
		Depth:   depth + 1,
	}))
	MustWriteEvents(t, db, events)
	fromLatest, err := db.EventPositionInTopology(ctx, events[len(events)-1].EventID())
	if err != nil {
		t.Fatalf("failed to get EventPositionInTopology: %s", err)
	}
	fromFork, err := db.EventPositionInTopology(ctx, events[len(events)-3].EventID()) // Message 2
	if err != nil {
		t.Fatalf("failed to get EventPositionInTopology for event: %s", err)
	}
	// head towards the beginning of time
	to := types.TopologyToken{}

	testCases := []struct {
		Name  string
		From  types.TopologyToken
		Limit int
		Wants []*types.HeaderedEvent
	}{
		{
			Name:  "Pagination over the whole fork",
			From:  fromLatest,
			Limit: 5,
			Wants: reversed(events[len(events)-5:]),
		},
		{
			Name:  "Paginating to the middle of the fork",
			From:  fromLatest,
			Limit: 2,
			Wants: reversed(events[len(events)-2:]),
		},
		{
			Name:  "Pagination FROM the middle of the fork",
			From:  fromFork,
			Limit: 3,
			Wants: reversed(events[len(events)-5 : len(events)-2]),
		},
	}

	for _, tc := range testCases {
		// backpaginate messages starting at the latest position.
		paginatedEvents, err := db.GetEventsInTopologicalRange(ctx, &tc.From, &to, testRoomID, tc.Limit, true)
		if err != nil {
			t.Fatalf("%s GetEventsInRange returned an error: %s", tc.Name, err)
		}
		gots := gomatrixserverlib.HeaderedToClientEvents(db.StreamEventsToEvents(&testUserDeviceA, paginatedEvents), gomatrixserverlib.FormatAll)
		assertEventsEqual(t, tc.Name, true, gots, tc.Wants)
	}
}

// The purpose of this test is to make sure that the query to pull out events is honouring the room ID correctly.
// It works by creating two rooms with the same events in them, then selecting events by topological range.
// Specifically, we know that events with the same depth but lower stream positions are selected, and it's possible
// that this check isn't using the room ID if the brackets are wrong in the SQL query.
func TestGetEventsInTopologicalRangeMultiRoom(t *testing.T) {
	t.Parallel()
	db := MustCreateDatabase(t)

	makeEvents := func(roomID string) (events []*types.HeaderedEvent) {
		events = append(events, MustCreateEvent(t, roomID, nil, &gomatrixserverlib.EventBuilder{
			Content:  []byte(fmt.Sprintf(`{"room_version":"4","creator":"%s"}`, testUserIDA)),
			Type:     "m.room.create",
			StateKey: &emptyStateKey,
			Sender:   testUserIDA,
			Depth:    int64(len(events) + 1),
		}))
		events = append(events, MustCreateEvent(t, roomID, []*types.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
			Content:  []byte(`{"membership":"join"}`),
			Type:     "m.room.member",
			StateKey: &testUserIDA,
			Sender:   testUserIDA,
			Depth:    int64(len(events) + 1),
		}))
		return
	}

	roomA := "!room_a:" + string(testOrigin)
	roomB := "!room_b:" + string(testOrigin)
	eventsA := makeEvents(roomA)
	eventsB := makeEvents(roomB)
	MustWriteEvents(t, db, eventsA)
	MustWriteEvents(t, db, eventsB)
	from, err := db.MaxTopologicalPosition(ctx, roomB)
	if err != nil {
		t.Fatalf("failed to get MaxTopologicalPosition: %s", err)
	}
	// head towards the beginning of time
	to := types.TopologyToken{}

	// Query using room B as room A was inserted first and hence A will have lower stream positions but identical depths,
	// allowing this bug to surface.
	paginatedEvents, err := db.GetEventsInTopologicalRange(ctx, &from, &to, roomB, 5, true)
	if err != nil {
		t.Fatalf("GetEventsInRange returned an error: %s", err)
	}
	gots := gomatrixserverlib.HeaderedToClientEvents(db.StreamEventsToEvents(&testUserDeviceA, paginatedEvents), gomatrixserverlib.FormatAll)
	assertEventsEqual(t, "", true, gots, reversed(eventsB))
}

// The purpose of this test is to make sure that events are returned in the right *order* when they have been inserted in a manner similar to
// how any kind of backfill operation will insert the events. This test inserts the SimpleRoom events in a manner similar to how backfill over
// federation would:
// - First inserts join event of test user C
// - Inserts chunks of history in strata e.g (25-30, 20-25, 15-20, 10-15, 5-10, 0-5).
// The test then does a backfill to ensure that the response is ordered correctly according to depth.
func TestGetEventsInRangeWithEventsInsertedLikeBackfill(t *testing.T) {
	t.Parallel()
	db := MustCreateDatabase(t)
	events, _ := SimpleRoom(t, testRoomID, testUserIDA, testUserIDB)

	// "federation" join
	userC := fmt.Sprintf("@radiance:%s", testOrigin)
	joinEvent := MustCreateEvent(t, testRoomID, []*types.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
		Content:  []byte(`{"membership":"join"}`),
		Type:     "m.room.member",
		StateKey: &userC,
		Sender:   userC,
		Depth:    int64(len(events) + 1),
	})
	MustWriteEvents(t, db, []*types.HeaderedEvent{joinEvent})

	// Sync will return this for the prev_batch
	from := topologyTokenBefore(t, db, joinEvent.EventID())

	// inject events in batches as if they were from backfill
	// e.g [1,2,3,4,5,6] => [4,5,6] , [1,2,3]
	chunkSize := 5
	for i := len(events); i >= 0; i -= chunkSize {
		start := i - chunkSize
		if start < 0 {
			start = 0
		}
		backfill := events[start:i]
		MustWriteEvents(t, db, backfill)
	}

	// head towards the beginning of time
	to := types.TopologyToken{}

	// starting at `from`, backpaginate to the beginning of time, asserting as we go.
	chunkSize = 3
	events = reversed(events)
	for i := 0; i < len(events); i += chunkSize {
		paginatedEvents, err := db.GetEventsInTopologicalRange(ctx, from, &to, testRoomID, chunkSize, true)
		if err != nil {
			t.Fatalf("GetEventsInRange returned an error: %s", err)
		}
		gots := gomatrixserverlib.HeaderedToClientEvents(db.StreamEventsToEvents(&testUserDeviceA, paginatedEvents), gomatrixserverlib.FormatAll)
		endi := i + chunkSize
		if endi > len(events) {
			endi = len(events)
		}
		assertEventsEqual(t, from.String(), true, gots, events[i:endi])
		from = topologyTokenBefore(t, db, paginatedEvents[len(paginatedEvents)-1].EventID())
	}
}
*/

func TestSendToDeviceBehaviour(t *testing.T) {
	t.Parallel()
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	deviceID := "one"
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := MustCreateDatabase(t, dbType)
		defer close()
		// At this point there should be no messages. We haven't sent anything
		// yet.

		WithSnapshot(t, db, func(snapshot storage.DatabaseTransaction) {
			_, events, err := snapshot.SendToDeviceUpdatesForSync(ctx, alice.ID, deviceID, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 0 {
				t.Fatal("first call should have no updates")
			}
		})

		// Try sending a message.
		streamPos, err := db.StoreNewSendForDeviceMessage(ctx, alice.ID, deviceID, gomatrixserverlib.SendToDeviceEvent{
			Sender:  bob.ID,
			Type:    "m.type",
			Content: json.RawMessage("{}"),
		})
		if err != nil {
			t.Fatal(err)
		}

		WithSnapshot(t, db, func(snapshot storage.DatabaseTransaction) {
			// At this point we should get exactly one message. We're sending the sync position
			// that we were given from the update and the send-to-device update will be updated
			// in the database to reflect that this was the sync position we sent the message at.
			var events []types.SendToDeviceEvent
			streamPos, events, err = snapshot.SendToDeviceUpdatesForSync(ctx, alice.ID, deviceID, 0, streamPos)
			if err != nil {
				t.Fatal(err)
			}
			if count := len(events); count != 1 {
				t.Fatalf("second call should have one update, got %d", count)
			}

			// At this point we should still have one message because we haven't progressed the
			// sync position yet. This is equivalent to the client failing to /sync and retrying
			// with the same position.
			streamPos, events, err = snapshot.SendToDeviceUpdatesForSync(ctx, alice.ID, deviceID, 0, streamPos)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 {
				t.Fatal("third call should have one update still")
			}
		})

		err = db.CleanSendToDeviceUpdates(context.Background(), alice.ID, deviceID, streamPos)
		if err != nil {
			return
		}

		WithSnapshot(t, db, func(snapshot storage.DatabaseTransaction) {
			// At this point we should now have no updates, because we've progressed the sync
			// position. Therefore the update from before will not be sent again.
			var events []types.SendToDeviceEvent
			_, events, err = snapshot.SendToDeviceUpdatesForSync(ctx, alice.ID, deviceID, streamPos, streamPos+10)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 0 {
				t.Fatal("fourth call should have no updates")
			}

			// At this point we should still have no updates, because no new updates have been
			// sent.
			_, events, err = snapshot.SendToDeviceUpdatesForSync(ctx, alice.ID, deviceID, streamPos, streamPos+10)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 0 {
				t.Fatal("fifth call should have no updates")
			}
		})

		// Send some more messages and verify the ordering is correct ("in order of arrival")
		var lastPos types.StreamPosition = 0
		for i := 0; i < 10; i++ {
			streamPos, err = db.StoreNewSendForDeviceMessage(ctx, alice.ID, deviceID, gomatrixserverlib.SendToDeviceEvent{
				Sender:  bob.ID,
				Type:    "m.type",
				Content: json.RawMessage(fmt.Sprintf(`{"count":%d}`, i)),
			})
			if err != nil {
				t.Fatal(err)
			}
			lastPos = streamPos
		}

		WithSnapshot(t, db, func(snapshot storage.DatabaseTransaction) {
			_, events, err := snapshot.SendToDeviceUpdatesForSync(ctx, alice.ID, deviceID, 0, lastPos)
			if err != nil {
				t.Fatalf("unable to get events: %v", err)
			}

			for i := 0; i < 10; i++ {
				want := json.RawMessage(fmt.Sprintf(`{"count":%d}`, i))
				got := events[i].Content
				if !bytes.Equal(got, want) {
					t.Fatalf("messages are out of order\nwant: %s\ngot: %s", string(want), string(got))
				}
			}
		})
	})
}

/*
func TestInviteBehaviour(t *testing.T) {
	db := MustCreateDatabase(t)
	inviteRoom1 := "!inviteRoom1:somewhere"
	inviteEvent1 := MustCreateEvent(t, inviteRoom1, nil, &gomatrixserverlib.EventBuilder{
		Content:  []byte(`{"membership":"invite"}`),
		Type:     "m.room.member",
		StateKey: &testUserIDA,
		Sender:   "@inviteUser1:somewhere",
	})
	inviteRoom2 := "!inviteRoom2:somewhere"
	inviteEvent2 := MustCreateEvent(t, inviteRoom2, nil, &gomatrixserverlib.EventBuilder{
		Content:  []byte(`{"membership":"invite"}`),
		Type:     "m.room.member",
		StateKey: &testUserIDA,
		Sender:   "@inviteUser2:somewhere",
	})
	for _, ev := range []*types.HeaderedEvent{inviteEvent1, inviteEvent2} {
		_, err := db.AddInviteEvent(ctx, ev)
		if err != nil {
			t.Fatalf("Failed to AddInviteEvent: %s", err)
		}
	}
	latest, err := db.SyncPosition(ctx)
	if err != nil {
		t.Fatalf("failed to get SyncPosition: %s", err)
	}
	// both invite events should appear in a new sync
	beforeRetireRes := types.NewResponse()
	beforeRetireRes, err = db.IncrementalSync(ctx, beforeRetireRes, testUserDeviceA, types.StreamingToken{}, latest, 0, false)
	if err != nil {
		t.Fatalf("IncrementalSync failed: %s", err)
	}
	assertInvitedToRooms(t, beforeRetireRes, []string{inviteRoom1, inviteRoom2})

	// retire one event: a fresh sync should just return 1 invite room
	if _, err = db.RetireInviteEvent(ctx, inviteEvent1.EventID()); err != nil {
		t.Fatalf("Failed to RetireInviteEvent: %s", err)
	}
	latest, err = db.SyncPosition(ctx)
	if err != nil {
		t.Fatalf("failed to get SyncPosition: %s", err)
	}
	res := types.NewResponse()
	res, err = db.IncrementalSync(ctx, res, testUserDeviceA, types.StreamingToken{}, latest, 0, false)
	if err != nil {
		t.Fatalf("IncrementalSync failed: %s", err)
	}
	assertInvitedToRooms(t, res, []string{inviteRoom2})

	// a sync after we have received both invites should result in a leave for the retired room
	res = types.NewResponse()
	res, err = db.IncrementalSync(ctx, res, testUserDeviceA, beforeRetireRes.NextBatch, latest, 0, false)
	if err != nil {
		t.Fatalf("IncrementalSync failed: %s", err)
	}
	assertInvitedToRooms(t, res, []string{})
	if _, ok := res.Rooms.Leave[inviteRoom1]; !ok {
		t.Fatalf("IncrementalSync: expected to see room left after it was retired but it wasn't")
	}
}

func assertInvitedToRooms(t *testing.T, res *types.Response, roomIDs []string) {
	t.Helper()
	if len(res.Rooms.Invite) != len(roomIDs) {
		t.Fatalf("got %d invited rooms, want %d", len(res.Rooms.Invite), len(roomIDs))
	}
	for _, roomID := range roomIDs {
		if _, ok := res.Rooms.Invite[roomID]; !ok {
			t.Fatalf("missing room ID %s", roomID)
		}
	}
}

func assertEventsEqual(t *testing.T, msg string, checkRoomID bool, gots []gomatrixserverlib.ClientEvent, wants []*types.HeaderedEvent) {
	t.Helper()
	if len(gots) != len(wants) {
		t.Fatalf("%s response returned %d events, want %d", msg, len(gots), len(wants))
	}
	for i := range gots {
		g := gots[i]
		w := wants[i]
		if g.EventID != w.EventID() {
			t.Errorf("%s event[%d] event_id mismatch: got %s want %s", msg, i, g.EventID, w.EventID())
		}
		if g.Sender != w.Sender() {
			t.Errorf("%s event[%d] sender mismatch: got %s want %s", msg, i, g.Sender, w.Sender())
		}
		if checkRoomID && g.RoomID != w.RoomID() {
			t.Errorf("%s event[%d] room_id mismatch: got %s want %s", msg, i, g.RoomID, w.RoomID())
		}
		if g.Type != w.Type() {
			t.Errorf("%s event[%d] event type mismatch: got %s want %s", msg, i, g.Type, w.Type())
		}
		if g.OriginServerTS != w.OriginServerTS() {
			t.Errorf("%s event[%d] origin_server_ts mismatch: got %v want %v", msg, i, g.OriginServerTS, w.OriginServerTS())
		}
		if string(g.Content) != string(w.Content()) {
			t.Errorf("%s event[%d] content mismatch: got %s want %s", msg, i, string(g.Content), string(w.Content()))
		}
		if string(g.Unsigned) != string(w.Unsigned()) {
			t.Errorf("%s event[%d] unsigned mismatch: got %s want %s", msg, i, string(g.Unsigned), string(w.Unsigned()))
		}
		if (g.StateKey == nil && w.StateKey() != nil) || (g.StateKey != nil && w.StateKey() == nil) {
			t.Errorf("%s event[%d] state_key [not] missing: got %v want %v", msg, i, g.StateKey, w.StateKey())
			continue
		}
		if g.StateKey != nil {
			if !w.StateKeyEquals(*g.StateKey) {
				t.Errorf("%s event[%d] state_key mismatch: got %s want %s", msg, i, *g.StateKey, *w.StateKey())
			}
		}
	}
}

func topologyTokenBefore(t *testing.T, db storage.Database, eventID string) *types.TopologyToken {
	tok, err := db.EventPositionInTopology(ctx, eventID)
	if err != nil {
		t.Fatalf("failed to get EventPositionInTopology: %s", err)
	}
	tok.Decrement()
	return &tok
}
*/

func pointer[t any](s t) *t {
	return &s
}

func TestRoomSummary(t *testing.T) {

	alice := test.NewUser(t)
	bob := test.NewUser(t)
	charlie := test.NewUser(t)

	// Create some dummy users
	moreUsers := []*test.User{}
	moreUserIDs := []string{}
	for i := 0; i < 10; i++ {
		u := test.NewUser(t)
		moreUsers = append(moreUsers, u)
		moreUserIDs = append(moreUserIDs, u.ID)
	}

	testCases := []struct {
		name             string
		wantSummary      *types.Summary
		additionalEvents func(t *testing.T, room *test.Room)
	}{
		{
			name:        "after initial creation",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(1), InvitedMemberCount: pointer(0), Heroes: []string{}},
		},
		{
			name:        "invited user",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(1), InvitedMemberCount: pointer(1), Heroes: []string{bob.ID}},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": "invite",
				}, test.WithStateKey(bob.ID))
			},
		},
		{
			name:        "invited user, but declined",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(1), InvitedMemberCount: pointer(0), Heroes: []string{bob.ID}},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": "invite",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "leave",
				}, test.WithStateKey(bob.ID))
			},
		},
		{
			name:        "joined user after invitation",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(2), InvitedMemberCount: pointer(0), Heroes: []string{bob.ID}},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": "invite",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
			},
		},
		{
			name:        "multiple joined user",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(3), InvitedMemberCount: pointer(0), Heroes: []string{charlie.ID, bob.ID}},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, charlie, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(charlie.ID))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
			},
		},
		{
			name:        "multiple joined/invited user",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(2), InvitedMemberCount: pointer(1), Heroes: []string{charlie.ID, bob.ID}},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": "invite",
				}, test.WithStateKey(charlie.ID))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
			},
		},
		{
			name:        "multiple joined/invited/left user",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(1), InvitedMemberCount: pointer(1), Heroes: []string{charlie.ID}},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, alice, spec.MRoomMember, map[string]interface{}{
					"membership": "invite",
				}, test.WithStateKey(charlie.ID))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "leave",
				}, test.WithStateKey(bob.ID))
			},
		},
		{
			name:        "leaving user after joining",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(1), InvitedMemberCount: pointer(0), Heroes: []string{bob.ID}},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "leave",
				}, test.WithStateKey(bob.ID))
			},
		},
		{
			name:        "many users", // heroes ordered by stream id
			wantSummary: &types.Summary{JoinedMemberCount: pointer(len(moreUserIDs) + 1), InvitedMemberCount: pointer(0), Heroes: moreUserIDs[:5]},
			additionalEvents: func(t *testing.T, room *test.Room) {
				for _, x := range moreUsers {
					room.CreateAndInsert(t, x, spec.MRoomMember, map[string]interface{}{
						"membership": "join",
					}, test.WithStateKey(x.ID))
				}
			},
		},
		{
			name:        "canonical alias set",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(2), InvitedMemberCount: pointer(0), Heroes: []string{}},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, alice, spec.MRoomCanonicalAlias, map[string]interface{}{
					"alias": "myalias",
				}, test.WithStateKey(""))
			},
		},
		{
			name:        "room name set",
			wantSummary: &types.Summary{JoinedMemberCount: pointer(2), InvitedMemberCount: pointer(0), Heroes: []string{}},
			additionalEvents: func(t *testing.T, room *test.Room) {
				room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]interface{}{
					"membership": "join",
				}, test.WithStateKey(bob.ID))
				room.CreateAndInsert(t, alice, spec.MRoomName, map[string]interface{}{
					"name": "my room name",
				}, test.WithStateKey(""))
			},
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := MustCreateDatabase(t, dbType)
		defer close()

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {

				r := test.NewRoom(t, alice)

				if tc.additionalEvents != nil {
					tc.additionalEvents(t, r)
				}

				// write the room before creating a transaction
				MustWriteEvents(t, db, r.Events())

				transaction, err := db.NewDatabaseTransaction(ctx)
				assert.NoError(t, err)
				defer transaction.Rollback()

				summary, err := transaction.GetRoomSummary(ctx, r.ID, alice.ID)
				assert.NoError(t, err)
				assert.Equal(t, tc.wantSummary, summary)
			})
		}
	})
}

func TestRecentEvents(t *testing.T) {
	alice := test.NewUser(t)
	room1 := test.NewRoom(t, alice)
	room2 := test.NewRoom(t, alice)
	roomIDs := []string{room1.ID, room2.ID}
	rooms := map[string]*test.Room{
		room1.ID: room1,
		room2.ID: room2,
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		filter := synctypes.DefaultRoomEventFilter()
		db, close := MustCreateDatabase(t, dbType)
		t.Cleanup(close)

		MustWriteEvents(t, db, room1.Events())
		MustWriteEvents(t, db, room2.Events())

		transaction, err := db.NewDatabaseTransaction(ctx)
		assert.NoError(t, err)
		defer transaction.Rollback()

		// get all recent events from 0 to 100 (we only created 5 events, so we should get 5 back)
		roomEvs, err := transaction.RecentEvents(ctx, roomIDs, types.Range{From: 0, To: 100}, &filter, true, true)
		assert.NoError(t, err)
		assert.Equal(t, len(roomEvs), 2, "unexpected recent events response")
		for _, recentEvents := range roomEvs {
			assert.Equal(t, 5, len(recentEvents.Events), "unexpected recent events for room")
		}

		// update the filter to only return one event
		filter.Limit = 1
		roomEvs, err = transaction.RecentEvents(ctx, roomIDs, types.Range{From: 0, To: 100}, &filter, true, true)
		assert.NoError(t, err)
		assert.Equal(t, len(roomEvs), 2, "unexpected recent events response")
		for roomID, recentEvents := range roomEvs {
			origEvents := rooms[roomID].Events()
			assert.Equal(t, true, recentEvents.Limited, "expected events to be limited")
			assert.Equal(t, 1, len(recentEvents.Events), "unexpected recent events for room")
			assert.Equal(t, origEvents[len(origEvents)-1].EventID(), recentEvents.Events[0].EventID())
		}

		// not chronologically ordered still returns the events in order (given ORDER BY id DESC)
		roomEvs, err = transaction.RecentEvents(ctx, roomIDs, types.Range{From: 0, To: 100}, &filter, false, true)
		assert.NoError(t, err)
		assert.Equal(t, len(roomEvs), 2, "unexpected recent events response")
		for roomID, recentEvents := range roomEvs {
			origEvents := rooms[roomID].Events()
			assert.Equal(t, true, recentEvents.Limited, "expected events to be limited")
			assert.Equal(t, 1, len(recentEvents.Events), "unexpected recent events for room")
			assert.Equal(t, origEvents[len(origEvents)-1].EventID(), recentEvents.Events[0].EventID())
		}
	})
}

type FakeQuerier struct {
	api.QuerySenderIDAPI
}

func (f *FakeQuerier) QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	return spec.NewUserID(string(senderID), true)
}

func TestRedaction(t *testing.T) {
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)

	redactedEvent := room.CreateAndInsert(t, alice, "m.room.message", map[string]interface{}{"body": "hi"})
	redactionEvent := room.CreateEvent(t, alice, spec.MRoomRedaction, map[string]string{"redacts": redactedEvent.EventID()})
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := MustCreateDatabase(t, dbType)
		t.Cleanup(close)
		MustWriteEvents(t, db, room.Events())

		err := db.RedactEvent(context.Background(), redactedEvent.EventID(), redactionEvent, &FakeQuerier{})
		if err != nil {
			t.Fatal(err)
		}

		evs, err := db.Events(context.Background(), []string{redactedEvent.EventID()})
		if err != nil {
			t.Fatal(err)
		}

		if len(evs) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evs))
		}

		// check a few fields which shouldn't be there in unsigned
		authEvs := gjson.GetBytes(evs[0].Unsigned(), "redacted_because.auth_events")
		if authEvs.Exists() {
			t.Error("unexpected auth_events in redacted event")
		}
		prevEvs := gjson.GetBytes(evs[0].Unsigned(), "redacted_because.prev_events")
		if prevEvs.Exists() {
			t.Error("unexpected auth_events in redacted event")
		}
		depth := gjson.GetBytes(evs[0].Unsigned(), "redacted_because.depth")
		if depth.Exists() {
			t.Error("unexpected auth_events in redacted event")
		}
	})
}
