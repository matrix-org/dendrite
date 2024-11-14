package internal

import (
	"context"
	"fmt"
	"math"
	"testing"

	rsapi "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"gotest.tools/v3/assert"
)

type mockHisVisRoomserverAPI struct {
	rsapi.RoomserverInternalAPI
	events []*types.HeaderedEvent
	roomID string
}

func (s *mockHisVisRoomserverAPI) QueryMembershipAtEvent(ctx context.Context, roomID spec.RoomID, eventIDs []string, senderID spec.SenderID) (map[string]*types.HeaderedEvent, error) {
	if roomID.String() == s.roomID {
		membershipMap := map[string]*types.HeaderedEvent{}

		for _, queriedEventID := range eventIDs {
			for _, event := range s.events {
				if event.EventID() == queriedEventID {
					membershipMap[queriedEventID] = event
				}
			}
		}

		return membershipMap, nil
	} else {
		return nil, fmt.Errorf("room not found: \"%v\"", roomID)
	}
}

func (s *mockHisVisRoomserverAPI) QuerySenderIDForUser(ctx context.Context, roomID spec.RoomID, userID spec.UserID) (*spec.SenderID, error) {
	senderID := spec.SenderIDFromUserID(userID)
	return &senderID, nil
}

func (s *mockHisVisRoomserverAPI) QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	userID := senderID.ToUserID()
	if userID == nil {
		return nil, fmt.Errorf("sender ID not user ID")
	}
	return userID, nil
}

type mockDB struct {
	storage.DatabaseTransaction
	// user ID -> membership (i.e. 'join', 'leave', etc.)
	currentMembership map[string]string
	roomID            string
}

func (s *mockDB) SelectMembershipForUser(ctx context.Context, roomID string, userID string, pos int64) (string, int64, error) {
	if roomID == s.roomID {
		membership, ok := s.currentMembership[userID]
		if !ok {
			return spec.Leave, math.MaxInt64, nil
		}
		return membership, math.MaxInt64, nil
	}

	return "", 0, fmt.Errorf("room not found: \"%v\"", roomID)
}

// Tests logic around history visibility boundaries
//
// Specifically that if a room's history visibility before or after a particular history visibility event
// allows them to see events (a boundary), then the history visibility event itself should be shown
// ( spec: https://spec.matrix.org/v1.8/client-server-api/#server-behaviour-5 )
//
// This also aims to emulate "Only see history_visibility changes on bounadries" in sytest/tests/30rooms/30history-visibility.pl
func Test_ApplyHistoryVisbility_Boundaries(t *testing.T) {
	ctx := context.Background()

	roomID := "!roomid:domain"

	creatorUserID := spec.NewUserIDOrPanic("@creator:domain", false)
	otherUserID := spec.NewUserIDOrPanic("@other:domain", false)
	roomVersion := gomatrixserverlib.RoomVersionV10
	roomVerImpl := gomatrixserverlib.MustGetRoomVersion(roomVersion)

	eventsJSON := []struct {
		id   string
		json string
	}{
		{id: "$create-event", json: fmt.Sprintf(`{
			"type": "m.room.create", "state_key": "",
			"room_id": "%v", "sender": "%v",
			"content": {"creator": "%v", "room_version": "%v"}
		}`, roomID, creatorUserID.String(), creatorUserID.String(), roomVersion)},
		{id: "$creator-joined", json: fmt.Sprintf(`{
			"type": "m.room.member", "state_key": "%v",
			"room_id": "%v", "sender": "%v",
			"content": {"membership": "join"}
		}`, creatorUserID.String(), roomID, creatorUserID.String())},
		{id: "$hisvis-1", json: fmt.Sprintf(`{
			"type": "m.room.history_visibility", "state_key": "",
			"room_id": "%v", "sender": "%v",
			"content": {"history_visibility": "shared"}
		}`, roomID, creatorUserID.String())},
		{id: "$msg-1", json: fmt.Sprintf(`{
			"type": "m.room.message",
			"room_id": "%v", "sender": "%v",
			"content": {"body": "1"}
		}`, roomID, creatorUserID.String())},
		{id: "$hisvis-2", json: fmt.Sprintf(`{
			"type": "m.room.history_visibility", "state_key": "",
			"room_id": "%v", "sender": "%v",
			"content": {"history_visibility": "joined"},
			"unsigned": {"prev_content": {"history_visibility": "shared"}}
		}`, roomID, creatorUserID.String())},
		{id: "$msg-2", json: fmt.Sprintf(`{
			"type": "m.room.message",
			"room_id": "%v", "sender": "%v",
			"content": {"body": "1"}
		}`, roomID, creatorUserID.String())},
		{id: "$hisvis-3", json: fmt.Sprintf(`{
			"type": "m.room.history_visibility", "state_key": "",
			"room_id": "%v", "sender": "%v",
			"content": {"history_visibility": "invited"},
			"unsigned": {"prev_content": {"history_visibility": "joined"}}
		}`, roomID, creatorUserID.String())},
		{id: "$msg-3", json: fmt.Sprintf(`{
			"type": "m.room.message",
			"room_id": "%v", "sender": "%v",
			"content": {"body": "2"}
		}`, roomID, creatorUserID.String())},
		{id: "$hisvis-4", json: fmt.Sprintf(`{
			"type": "m.room.history_visibility", "state_key": "",
			"room_id": "%v", "sender": "%v",
			"content": {"history_visibility": "shared"},
			"unsigned": {"prev_content": {"history_visibility": "invited"}}
		}`, roomID, creatorUserID.String())},
		{id: "$msg-4", json: fmt.Sprintf(`{
			"type": "m.room.message",
			"room_id": "%v", "sender": "%v",
			"content": {"body": "3"}
		}`, roomID, creatorUserID.String())},
		{id: "$other-joined", json: fmt.Sprintf(`{
			"type": "m.room.member", "state_key": "%v",
			"room_id": "%v", "sender": "%v",
			"content": {"membership": "join"}
		}`, otherUserID.String(), roomID, otherUserID.String())},
	}

	events := make([]*types.HeaderedEvent, len(eventsJSON))

	hisVis := gomatrixserverlib.HistoryVisibilityShared

	for i, eventJSON := range eventsJSON {
		pdu, err := roomVerImpl.NewEventFromTrustedJSONWithEventID(eventJSON.id, []byte(eventJSON.json), false)
		if err != nil {
			t.Fatalf("failed to prepare event %s for test: %s", eventJSON.id, err.Error())
		}
		events[i] = &types.HeaderedEvent{PDU: pdu}

		// 'Visibility' should be the visibility of the room just before this event was sent
		// (according to processRoomEvent in roomserver/internal/input/input_events.go)
		events[i].Visibility = hisVis
		if pdu.Type() == spec.MRoomHistoryVisibility {
			newHisVis, err := pdu.HistoryVisibility()
			if err != nil {
				t.Fatalf("failed to prepare history visibility event: %s", err.Error())
			}
			hisVis = newHisVis
		}
	}

	rsAPI := &mockHisVisRoomserverAPI{
		events: events,
		roomID: roomID,
	}
	syncDB := &mockDB{
		roomID: roomID,
		currentMembership: map[string]string{
			creatorUserID.String(): spec.Join,
			otherUserID.String():   spec.Join,
		},
	}

	filteredEvents, err := ApplyHistoryVisibilityFilter(ctx, syncDB, rsAPI, events, nil, otherUserID, "hisVisTest")
	if err != nil {
		t.Fatalf("ApplyHistoryVisibility returned non-nil error: %s", err.Error())
	}

	filteredEventIDs := make([]string, len(filteredEvents))
	for i, event := range filteredEvents {
		filteredEventIDs[i] = event.EventID()
	}

	assert.DeepEqual(t,
		[]string{
			"$create-event",   // Always see m.room.create
			"$creator-joined", // Always see membership
			"$hisvis-1",       // Sets room to shared (technically the room is already shared since shared is default)
			"$msg-1",          // Room currently 'shared'
			"$hisvis-2",       // Room changed from 'shared' to 'joined', so boundary event and should be shared
			// Other events hidden, as other is not joined yet
			// hisvis-3 is also hidden, as it changes from joined to invited, neither of which is visible to other
			"$hisvis-4",     // Changes from 'invited' to 'shared', so is a boundary event and visible
			"$msg-4",        // Room is 'shared', so visible
			"$other-joined", // other's membership
		},
		filteredEventIDs,
	)
}
