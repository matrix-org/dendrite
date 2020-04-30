package storage_test

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	ctx             = context.Background()
	emptyStateKey   = ""
	testOrigin      = gomatrixserverlib.ServerName("hollow.knight")
	testRoomID      = fmt.Sprintf("!hallownest:%s", testOrigin)
	testUserIDA     = fmt.Sprintf("@hornet:%s", testOrigin)
	testUserIDB     = fmt.Sprintf("@paleking:%s", testOrigin)
	testUserDeviceA = authtypes.Device{
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

func MustCreateEvent(t *testing.T, roomID string, prevs []gomatrixserverlib.HeaderedEvent, b *gomatrixserverlib.EventBuilder) gomatrixserverlib.HeaderedEvent {
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
	db, err := sqlite3.NewSyncServerDatasource("file::memory:")
	if err != nil {
		t.Fatalf("NewSyncServerDatasource returned %s", err)
	}
	return db
}

// Create a list of events which include a create event, join event and some messages.
func SimpleRoom(t *testing.T, roomID, userA, userB string) (msgs []gomatrixserverlib.HeaderedEvent, state []gomatrixserverlib.HeaderedEvent) {
	var events []gomatrixserverlib.HeaderedEvent
	events = append(events, MustCreateEvent(t, roomID, nil, &gomatrixserverlib.EventBuilder{
		Content:  []byte(fmt.Sprintf(`{"room_version":"4","creator":"%s"}`, userA)),
		Type:     "m.room.create",
		StateKey: &emptyStateKey,
		Sender:   userA,
		Depth:    int64(len(events) + 1),
	}))
	state = append(state, events[len(events)-1])
	events = append(events, MustCreateEvent(t, roomID, []gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
		Content:  []byte(fmt.Sprintf(`{"membership":"join"}`)),
		Type:     "m.room.member",
		StateKey: &userA,
		Sender:   userA,
		Depth:    int64(len(events) + 1),
	}))
	state = append(state, events[len(events)-1])
	for i := 0; i < 10; i++ {
		events = append(events, MustCreateEvent(t, roomID, []gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
			Content: []byte(fmt.Sprintf(`{"body":"Message A %d"}`, i+1)),
			Type:    "m.room.message",
			Sender:  userA,
			Depth:   int64(len(events) + 1),
		}))
	}
	events = append(events, MustCreateEvent(t, roomID, []gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
		Content:  []byte(fmt.Sprintf(`{"membership":"join"}`)),
		Type:     "m.room.member",
		StateKey: &userB,
		Sender:   userB,
		Depth:    int64(len(events) + 1),
	}))
	state = append(state, events[len(events)-1])
	for i := 0; i < 10; i++ {
		events = append(events, MustCreateEvent(t, roomID, []gomatrixserverlib.HeaderedEvent{events[len(events)-1]}, &gomatrixserverlib.EventBuilder{
			Content: []byte(fmt.Sprintf(`{"body":"Message B %d"}`, i+1)),
			Type:    "m.room.message",
			Sender:  userB,
			Depth:   int64(len(events) + 1),
		}))
	}

	return events, state
}

func MustWriteEvents(t *testing.T, db storage.Database, events []gomatrixserverlib.HeaderedEvent) (positions []types.StreamPosition) {
	for _, ev := range events {
		var addStateEvents []gomatrixserverlib.HeaderedEvent
		var addStateEventIDs []string
		var removeStateEventIDs []string
		if ev.StateKey() != nil {
			addStateEvents = append(addStateEvents, ev)
			addStateEventIDs = append(addStateEventIDs, ev.EventID())
		}
		pos, err := db.WriteEvent(ctx, &ev, addStateEvents, addStateEventIDs, removeStateEventIDs, nil, false)
		if err != nil {
			t.Fatalf("WriteEvent failed: %s", err)
		}
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
		WantTimeline []gomatrixserverlib.HeaderedEvent
		WantState    []gomatrixserverlib.HeaderedEvent
	}{
		// The purpose of this test is to make sure that incremental syncs are including up to the latest events.
		// It's a basic sanity test that sync works. It creates a `since` token that is on the penultimate event.
		// It makes sure the response includes the final event.
		{
			Name: "IncrementalSync penultimate",
			DoSync: func() (*types.Response, error) {
				from := types.NewPaginationTokenFromTypeAndPosition( // pretend we are at the penultimate event
					types.PaginationTokenTypeStream, positions[len(positions)-2], types.StreamPosition(0),
				)
				return db.IncrementalSync(ctx, testUserDeviceA, *from, latest, 5, false)
			},
			WantTimeline: events[len(events)-1:],
		},
		// The purpose of this test is to check that passing a `numRecentEventsPerRoom` correctly limits the
		// number of returned events. This is critical for big rooms hence the test here.
		{
			Name: "IncrementalSync limited",
			DoSync: func() (*types.Response, error) {
				from := types.NewPaginationTokenFromTypeAndPosition( // pretend we are 10 events behind
					types.PaginationTokenTypeStream, positions[len(positions)-11], types.StreamPosition(0),
				)
				// limit is set to 5
				return db.IncrementalSync(ctx, testUserDeviceA, *from, latest, 5, false)
			},
			// want the last 5 events, NOT the last 10.
			WantTimeline: events[len(events)-5:],
		},
		// The purpose of this test is to check that CompleteSync returns all the current state as well as
		// honouring the `numRecentEventsPerRoom` value
		{
			Name: "CompleteSync limited",
			DoSync: func() (*types.Response, error) {
				// limit set to 5
				return db.CompleteSync(ctx, testUserIDA, 5)
			},
			// want the last 5 events, NOT the last 10.
			WantTimeline: events[len(events)-5:],
			// want all state for the room
			WantState: state,
		},
		// The purpose of this test is to check that CompleteSync can return everything with a high enough
		// `numRecentEventsPerRoom`.
		{
			Name: "CompleteSync",
			DoSync: func() (*types.Response, error) {
				return db.CompleteSync(ctx, testUserIDA, len(events)+1)
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
			next := types.NewPaginationTokenFromTypeAndPosition(types.PaginationTokenTypeStream, latest.PDUPosition, latest.EDUTypingPosition)
			if res.NextBatch != next.String() {
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
	to := types.NewPaginationTokenFromTypeAndPosition(types.PaginationTokenTypeTopology, 0, 0)

	// backpaginate 5 messages starting at the latest position.
	paginatedEvents, err := db.GetEventsInRange(ctx, &latest, to, testRoomID, 5, true)
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
	latest, err := db.MaxTopologicalPosition(ctx, testRoomID)
	if err != nil {
		t.Fatalf("failed to get MaxTopologicalPosition: %s", err)
	}
	from := types.NewPaginationTokenFromTypeAndPosition(types.PaginationTokenTypeTopology, latest, 0)
	// head towards the beginning of time
	to := types.NewPaginationTokenFromTypeAndPosition(types.PaginationTokenTypeTopology, 0, 0)

	// backpaginate 5 messages starting at the latest position.
	paginatedEvents, err := db.GetEventsInRange(ctx, from, to, testRoomID, 5, true)
	if err != nil {
		t.Fatalf("GetEventsInRange returned an error: %s", err)
	}
	gots := gomatrixserverlib.HeaderedToClientEvents(db.StreamEventsToEvents(&testUserDeviceA, paginatedEvents), gomatrixserverlib.FormatAll)
	assertEventsEqual(t, "", true, gots, reversed(events[len(events)-5:]))
}

func assertEventsEqual(t *testing.T, msg string, checkRoomID bool, gots []gomatrixserverlib.ClientEvent, wants []gomatrixserverlib.HeaderedEvent) {
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
			t.Fatalf("%s event[%d] state_key [not] missing: got %v want %v", msg, i, g.StateKey, w.StateKey())
		}
		if g.StateKey != nil {
			if !w.StateKeyEquals(*g.StateKey) {
				t.Errorf("%s event[%d] state_key mismatch: got %s want %s", msg, i, *g.StateKey, *w.StateKey())
			}
		}
	}
}

func reversed(in []gomatrixserverlib.HeaderedEvent) []gomatrixserverlib.HeaderedEvent {
	out := make([]gomatrixserverlib.HeaderedEvent, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[len(in)-i-1]
	}
	return out
}
