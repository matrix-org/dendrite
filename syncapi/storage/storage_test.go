package storage_test

// TODO: Fix these tests
/*
import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	ctx             = context.Background()
	emptyStateKey   = ""
	testOrigin      = gomatrixserverlib.ServerName("hollow.knight")
	testRoomID      = fmt.Sprintf("!hallownest:%s", testOrigin)
	testUserIDA     = fmt.Sprintf("@hornet:%s", testOrigin)
	testUserIDB     = fmt.Sprintf("@paleking:%s", testOrigin)
	testUserDeviceA = userapi.Device{
		UserID:      testUserIDA,
		ID:          "device_id_A",
		DisplayName: "Device A",
	}
	testRoomVersion = gomatrixserverlib.RoomVersionV4
	testKeyID       = gomatrixserverlib.KeyID("ed25519:storage_test")
	testPrivateKey  = ed25519.NewKeyFromSeed([]byte{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
	})
)

func MustCreateEvent(t *testing.T, roomID string, prevs []*gomatrixserverlib.HeaderedEvent, b *gomatrixserverlib.EventBuilder) *gomatrixserverlib.HeaderedEvent {
	b.RoomID = roomID
	if prevs != nil {
		prevIDs := make([]string, len(prevs))
		for i := range prevs {
			prevIDs[i] = prevs[i].EventID()
		}
		b.PrevEvents = prevIDs
	}
	e, err := b.Build(time.Now(), testOrigin, testKeyID, testPrivateKey, testRoomVersion)
	if err != nil {
		t.Fatalf("failed to build event: %s", err)
	}
	return e.Headered(testRoomVersion)
}

func MustCreateDatabase(t *testing.T) storage.Database {
	dbname := fmt.Sprintf("test_%s.db", t.Name())
	if _, err := os.Stat(dbname); err == nil {
		if err = os.Remove(dbname); err != nil {
			t.Fatalf("tried to delete stale test database but failed: %s", err)
		}
	}
	db, err := sqlite3.NewDatabase(&config.DatabaseOptions{
		ConnectionString: config.DataSource(fmt.Sprintf("file:%s", dbname)),
	})
	if err != nil {
		t.Fatalf("NewSyncServerDatasource returned %s", err)
	}
	return db
}

// Create a list of events which include a create event, join event and some messages.
func SimpleRoom(t *testing.T, roomID, userA, userB string) (msgs []*gomatrixserverlib.HeaderedEvent, state []*gomatrixserverlib.HeaderedEvent) {
	var events []*gomatrixserverlib.HeaderedEvent
	events = append(events, MustCreateEvent(t, roomID, nil, &gomatrixserverlib.EventBuilder{
		Content:  []byte(fmt.Sprintf(`{"room_version":"4","creator":"%s"}`, userA)),
		Type:     "m.room.create",
		StateKey: &emptyStateKey,
		Sender:   userA,
		Depth:    int64(len(events) + 1),
	}))
	state = append(state, events[len(events)-1])
	events = append(events, MustCreateEvent(t, roomID, []*gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
		Content:  []byte(`{"membership":"join"}`),
		Type:     "m.room.member",
		StateKey: &userA,
		Sender:   userA,
		Depth:    int64(len(events) + 1),
	}))
	state = append(state, events[len(events)-1])
	for i := 0; i < 10; i++ {
		events = append(events, MustCreateEvent(t, roomID, []*gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
			Content: []byte(fmt.Sprintf(`{"body":"Message A %d"}`, i+1)),
			Type:    "m.room.message",
			Sender:  userA,
			Depth:   int64(len(events) + 1),
		}))
	}
	events = append(events, MustCreateEvent(t, roomID, []*gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
		Content:  []byte(`{"membership":"join"}`),
		Type:     "m.room.member",
		StateKey: &userB,
		Sender:   userB,
		Depth:    int64(len(events) + 1),
	}))
	state = append(state, events[len(events)-1])
	for i := 0; i < 10; i++ {
		events = append(events, MustCreateEvent(t, roomID, []*gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
			Content: []byte(fmt.Sprintf(`{"body":"Message B %d"}`, i+1)),
			Type:    "m.room.message",
			Sender:  userB,
			Depth:   int64(len(events) + 1),
		}))
	}

	return events, state
}

func MustWriteEvents(t *testing.T, db storage.Database, events []*gomatrixserverlib.HeaderedEvent) (positions []types.StreamPosition) {
	for _, ev := range events {
		var addStateEvents []*gomatrixserverlib.HeaderedEvent
		var addStateEventIDs []string
		var removeStateEventIDs []string
		if ev.StateKey() != nil {
			addStateEvents = append(addStateEvents, ev)
			addStateEventIDs = append(addStateEventIDs, ev.EventID())
		}
		pos, err := db.WriteEvent(ctx, ev, addStateEvents, addStateEventIDs, removeStateEventIDs, nil, false)
		if err != nil {
			t.Fatalf("WriteEvent failed: %s", err)
		}
		fmt.Println("Event ID", ev.EventID(), " spos=", pos, "depth=", ev.Depth())
		positions = append(positions, pos)
	}
	return
}

func TestWriteEvents(t *testing.T) {
	t.Parallel()
	db := MustCreateDatabase(t)
	events, _ := SimpleRoom(t, testRoomID, testUserIDA, testUserIDB)
	MustWriteEvents(t, db, events)
}

// These tests assert basic functionality of the IncrementalSync and CompleteSync functions.
func TestSyncResponse(t *testing.T) {
	t.Parallel()
	db := MustCreateDatabase(t)
	events, state := SimpleRoom(t, testRoomID, testUserIDA, testUserIDB)
	positions := MustWriteEvents(t, db, events)
	latest, err := db.SyncPosition(ctx)
	if err != nil {
		t.Fatalf("failed to get SyncPosition: %s", err)
	}

	testCases := []struct {
		Name         string
		DoSync       func() (*types.Response, error)
		WantTimeline []*gomatrixserverlib.HeaderedEvent
		WantState    []*gomatrixserverlib.HeaderedEvent
	}{
		// The purpose of this test is to make sure that incremental syncs are including up to the latest events.
		// It's a basic sanity test that sync works. It creates a `since` token that is on the penultimate event.
		// It makes sure the response includes the final event.
		{
			Name: "IncrementalSync penultimate",
			DoSync: func() (*types.Response, error) {
				from := types.StreamingToken{ // pretend we are at the penultimate event
					PDUPosition: positions[len(positions)-2],
				}
				res := types.NewResponse()
				return db.IncrementalSync(ctx, res, testUserDeviceA, from, latest, 5, false)
			},
			WantTimeline: events[len(events)-1:],
		},
		// The purpose of this test is to check that passing a `numRecentEventsPerRoom` correctly limits the
		// number of returned events. This is critical for big rooms hence the test here.
		{
			Name: "IncrementalSync limited",
			DoSync: func() (*types.Response, error) {
				from := types.StreamingToken{ // pretend we are 10 events behind
					PDUPosition: positions[len(positions)-11],
				}
				res := types.NewResponse()
				// limit is set to 5
				return db.IncrementalSync(ctx, res, testUserDeviceA, from, latest, 5, false)
			},
			// want the last 5 events, NOT the last 10.
			WantTimeline: events[len(events)-5:],
		},
		// The purpose of this test is to check that CompleteSync returns all the current state as well as
		// honouring the `numRecentEventsPerRoom` value
		{
			Name: "CompleteSync limited",
			DoSync: func() (*types.Response, error) {
				res := types.NewResponse()
				// limit set to 5
				return db.CompleteSync(ctx, res, testUserDeviceA, 5)
			},
			// want the last 5 events
			WantTimeline: events[len(events)-5:],
			// want all state for the room
			WantState: state,
		},
		// The purpose of this test is to check that CompleteSync can return everything with a high enough
		// `numRecentEventsPerRoom`.
		{
			Name: "CompleteSync",
			DoSync: func() (*types.Response, error) {
				res := types.NewResponse()
				return db.CompleteSync(ctx, res, testUserDeviceA, len(events)+1)
			},
			WantTimeline: events,
			// We want no state at all as that field in /sync is the delta between the token (beginning of time)
			// and the START of the timeline.
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(st *testing.T) {
			res, err := tc.DoSync()
			if err != nil {
				st.Fatalf("failed to do sync: %s", err)
			}
			next := types.StreamingToken{
				PDUPosition:          latest.PDUPosition,
				TypingPosition:       latest.TypingPosition,
				ReceiptPosition:      latest.ReceiptPosition,
				SendToDevicePosition: latest.SendToDevicePosition,
			}
			if res.NextBatch.String() != next.String() {
				st.Errorf("NextBatch got %s want %s", res.NextBatch, next.String())
			}
			roomRes, ok := res.Rooms.Join[testRoomID]
			if !ok {
				st.Fatalf("IncrementalSync response missing room %s - response: %+v", testRoomID, res)
			}
			assertEventsEqual(st, "state for "+testRoomID, false, roomRes.State.Events, tc.WantState)
			assertEventsEqual(st, "timeline for "+testRoomID, false, roomRes.Timeline.Events, tc.WantTimeline)
		})
	}
}

func TestGetEventsInRangeWithPrevBatch(t *testing.T) {
	t.Parallel()
	db := MustCreateDatabase(t)
	events, _ := SimpleRoom(t, testRoomID, testUserIDA, testUserIDB)
	positions := MustWriteEvents(t, db, events)
	latest, err := db.SyncPosition(ctx)
	if err != nil {
		t.Fatalf("failed to get SyncPosition: %s", err)
	}
	from := types.StreamingToken{
		PDUPosition: positions[len(positions)-2],
	}

	res := types.NewResponse()
	res, err = db.IncrementalSync(ctx, res, testUserDeviceA, from, latest, 5, false)
	if err != nil {
		t.Fatalf("failed to IncrementalSync with latest token")
	}
	roomRes, ok := res.Rooms.Join[testRoomID]
	if !ok {
		t.Fatalf("IncrementalSync response missing room %s - response: %+v", testRoomID, res)
	}
	// returns the last event "Message 10"
	assertEventsEqual(t, "IncrementalSync Timeline", false, roomRes.Timeline.Events, reversed(events[len(events)-1:]))

	prev := roomRes.Timeline.PrevBatch.String()
	if prev == "" {
		t.Fatalf("IncrementalSync expected prev_batch token")
	}
	prevBatchToken, err := types.NewTopologyTokenFromString(prev)
	if err != nil {
		t.Fatalf("failed to NewTopologyTokenFromString : %s", err)
	}
	// backpaginate 5 messages starting at the latest position.
	// head towards the beginning of time
	to := types.TopologyToken{}
	paginatedEvents, err := db.GetEventsInTopologicalRange(ctx, &prevBatchToken, &to, testRoomID, 5, true)
	if err != nil {
		t.Fatalf("GetEventsInRange returned an error: %s", err)
	}
	gots := gomatrixserverlib.HeaderedToClientEvents(db.StreamEventsToEvents(&testUserDeviceA, paginatedEvents), gomatrixserverlib.FormatAll)
	assertEventsEqual(t, "", true, gots, reversed(events[len(events)-6:len(events)-1]))
}

// The purpose of this test is to ensure that backfill does indeed go backwards, using a stream token.
func TestGetEventsInRangeWithStreamToken(t *testing.T) {
	t.Parallel()
	db := MustCreateDatabase(t)
	events, _ := SimpleRoom(t, testRoomID, testUserIDA, testUserIDB)
	MustWriteEvents(t, db, events)
	latest, err := db.SyncPosition(ctx)
	if err != nil {
		t.Fatalf("failed to get SyncPosition: %s", err)
	}
	// head towards the beginning of time
	to := types.StreamingToken{}

	// backpaginate 5 messages starting at the latest position.
	paginatedEvents, err := db.GetEventsInStreamingRange(ctx, &latest, &to, testRoomID, 5, true)
	if err != nil {
		t.Fatalf("GetEventsInRange returned an error: %s", err)
	}
	gots := gomatrixserverlib.HeaderedToClientEvents(db.StreamEventsToEvents(&testUserDeviceA, paginatedEvents), gomatrixserverlib.FormatAll)
	assertEventsEqual(t, "", true, gots, reversed(events[len(events)-5:]))
}

// The purpose of this test is to ensure that backfill does indeed go backwards, using a topology token
func TestGetEventsInRangeWithTopologyToken(t *testing.T) {
	t.Parallel()
	db := MustCreateDatabase(t)
	events, _ := SimpleRoom(t, testRoomID, testUserIDA, testUserIDB)
	MustWriteEvents(t, db, events)
	from, err := db.MaxTopologicalPosition(ctx, testRoomID)
	if err != nil {
		t.Fatalf("failed to get MaxTopologicalPosition: %s", err)
	}
	// head towards the beginning of time
	to := types.TopologyToken{}

	// backpaginate 5 messages starting at the latest position.
	paginatedEvents, err := db.GetEventsInTopologicalRange(ctx, &from, &to, testRoomID, 5, true)
	if err != nil {
		t.Fatalf("GetEventsInRange returned an error: %s", err)
	}
	gots := gomatrixserverlib.HeaderedToClientEvents(db.StreamEventsToEvents(&testUserDeviceA, paginatedEvents), gomatrixserverlib.FormatAll)
	assertEventsEqual(t, "", true, gots, reversed(events[len(events)-5:]))
}

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

	var events []*gomatrixserverlib.HeaderedEvent
	events = append(events, MustCreateEvent(t, testRoomID, nil, &gomatrixserverlib.EventBuilder{
		Content:  []byte(fmt.Sprintf(`{"room_version":"4","creator":"%s"}`, testUserIDA)),
		Type:     "m.room.create",
		StateKey: &emptyStateKey,
		Sender:   testUserIDA,
		Depth:    int64(len(events) + 1),
	}))
	events = append(events, MustCreateEvent(t, testRoomID, []*gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
		Content:  []byte(`{"membership":"join"}`),
		Type:     "m.room.member",
		StateKey: &testUserIDA,
		Sender:   testUserIDA,
		Depth:    int64(len(events) + 1),
	}))
	// fork the dag into three, same prev_events and depth
	parent := []*gomatrixserverlib.HeaderedEvent{events[len(events)-1]}
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
		Wants []*gomatrixserverlib.HeaderedEvent
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

	makeEvents := func(roomID string) (events []*gomatrixserverlib.HeaderedEvent) {
		events = append(events, MustCreateEvent(t, roomID, nil, &gomatrixserverlib.EventBuilder{
			Content:  []byte(fmt.Sprintf(`{"room_version":"4","creator":"%s"}`, testUserIDA)),
			Type:     "m.room.create",
			StateKey: &emptyStateKey,
			Sender:   testUserIDA,
			Depth:    int64(len(events) + 1),
		}))
		events = append(events, MustCreateEvent(t, roomID, []*gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
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
	joinEvent := MustCreateEvent(t, testRoomID, []*gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
		Content:  []byte(`{"membership":"join"}`),
		Type:     "m.room.member",
		StateKey: &userC,
		Sender:   userC,
		Depth:    int64(len(events) + 1),
	})
	MustWriteEvents(t, db, []*gomatrixserverlib.HeaderedEvent{joinEvent})

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

func TestSendToDeviceBehaviour(t *testing.T) {
	//t.Parallel()
	db := MustCreateDatabase(t)

	// At this point there should be no messages. We haven't sent anything
	// yet.
	_, events, updates, deletions, err := db.SendToDeviceUpdatesForSync(ctx, "alice", "one", types.StreamingToken{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 || len(updates) != 0 || len(deletions) != 0 {
		t.Fatal("first call should have no updates")
	}
	err = db.CleanSendToDeviceUpdates(context.Background(), updates, deletions, types.StreamingToken{})
	if err != nil {
		return
	}

	// Try sending a message.
	streamPos, err := db.StoreNewSendForDeviceMessage(ctx, "alice", "one", gomatrixserverlib.SendToDeviceEvent{
		Sender:  "bob",
		Type:    "m.type",
		Content: json.RawMessage("{}"),
	})
	if err != nil {
		t.Fatal(err)
	}

	// At this point we should get exactly one message. We're sending the sync position
	// that we were given from the update and the send-to-device update will be updated
	// in the database to reflect that this was the sync position we sent the message at.
	_, events, updates, deletions, err = db.SendToDeviceUpdatesForSync(ctx, "alice", "one", types.StreamingToken{SendToDevicePosition: streamPos})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || len(updates) != 1 || len(deletions) != 0 {
		t.Fatal("second call should have one update")
	}
	err = db.CleanSendToDeviceUpdates(context.Background(), updates, deletions, types.StreamingToken{SendToDevicePosition: streamPos})
	if err != nil {
		return
	}

	// At this point we should still have one message because we haven't progressed the
	// sync position yet. This is equivalent to the client failing to /sync and retrying
	// with the same position.
	_, events, updates, deletions, err = db.SendToDeviceUpdatesForSync(ctx, "alice", "one", types.StreamingToken{SendToDevicePosition: streamPos})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || len(updates) != 0 || len(deletions) != 0 {
		t.Fatal("third call should have one update still")
	}
	err = db.CleanSendToDeviceUpdates(context.Background(), updates, deletions, types.StreamingToken{SendToDevicePosition: streamPos})
	if err != nil {
		return
	}

	// At this point we should now have no updates, because we've progressed the sync
	// position. Therefore the update from before will not be sent again.
	_, events, updates, deletions, err = db.SendToDeviceUpdatesForSync(ctx, "alice", "one", types.StreamingToken{SendToDevicePosition: streamPos + 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 || len(updates) != 0 || len(deletions) != 1 {
		t.Fatal("fourth call should have no updates")
	}
	err = db.CleanSendToDeviceUpdates(context.Background(), updates, deletions, types.StreamingToken{SendToDevicePosition: streamPos + 1})
	if err != nil {
		return
	}

	// At this point we should still have no updates, because no new updates have been
	// sent.
	_, events, updates, deletions, err = db.SendToDeviceUpdatesForSync(ctx, "alice", "one", types.StreamingToken{SendToDevicePosition: streamPos + 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 || len(updates) != 0 || len(deletions) != 0 {
		t.Fatal("fifth call should have no updates")
	}
}

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
	for _, ev := range []*gomatrixserverlib.HeaderedEvent{inviteEvent1, inviteEvent2} {
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

func assertEventsEqual(t *testing.T, msg string, checkRoomID bool, gots []gomatrixserverlib.ClientEvent, wants []*gomatrixserverlib.HeaderedEvent) {
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

func reversed(in []*gomatrixserverlib.HeaderedEvent) []*gomatrixserverlib.HeaderedEvent {
	out := make([]*gomatrixserverlib.HeaderedEvent, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[len(in)-i-1]
	}
	return out
}
*/
