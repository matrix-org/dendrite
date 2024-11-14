package routing

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	rsapi "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	uapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"gotest.tools/v3/assert"
)

// Mock roomserver API for testing
//
// Currently pretty specialised for the pseudo ID test, so will need
// editing if future (other) sendevent tests are using this.
type sendEventTestRoomserverAPI struct {
	rsapi.ClientRoomserverAPI
	t           *testing.T
	roomIDStr   string
	roomVersion gomatrixserverlib.RoomVersion
	roomState   []*types.HeaderedEvent

	// userID -> room key
	senderMapping map[string]ed25519.PrivateKey

	savedInputRoomEvents []rsapi.InputRoomEvent
}

func (s *sendEventTestRoomserverAPI) QueryRoomVersionForRoom(ctx context.Context, roomID string) (gomatrixserverlib.RoomVersion, error) {
	if roomID == s.roomIDStr {
		return s.roomVersion, nil
	} else {
		s.t.Logf("room version queried for %s", roomID)
		return "", fmt.Errorf("unknown room")
	}
}

func (s *sendEventTestRoomserverAPI) QueryCurrentState(ctx context.Context, req *rsapi.QueryCurrentStateRequest, res *rsapi.QueryCurrentStateResponse) error {
	res.StateEvents = map[gomatrixserverlib.StateKeyTuple]*types.HeaderedEvent{}
	for _, stateKeyTuple := range req.StateTuples {
		for _, stateEv := range s.roomState {
			if stateEv.Type() == stateKeyTuple.EventType && stateEv.StateKey() != nil && *stateEv.StateKey() == stateKeyTuple.StateKey {
				res.StateEvents[stateKeyTuple] = stateEv
			}
		}
	}
	return nil
}

func (s *sendEventTestRoomserverAPI) QueryLatestEventsAndState(ctx context.Context, req *rsapi.QueryLatestEventsAndStateRequest, res *rsapi.QueryLatestEventsAndStateResponse) error {
	if req.RoomID == s.roomIDStr {
		res.RoomExists = true
		res.RoomVersion = s.roomVersion

		res.StateEvents = make([]*types.HeaderedEvent, len(s.roomState))
		copy(res.StateEvents, s.roomState)

		res.LatestEvents = []string{}
		res.Depth = 1
		return nil
	} else {
		s.t.Logf("room event/state queried for %s", req.RoomID)
		return fmt.Errorf("unknown room")
	}

}

func (s *sendEventTestRoomserverAPI) QuerySenderIDForUser(
	ctx context.Context,
	roomID spec.RoomID,
	userID spec.UserID,
) (*spec.SenderID, error) {
	if roomID.String() == s.roomIDStr {
		if s.roomVersion == gomatrixserverlib.RoomVersionPseudoIDs {
			roomKey, ok := s.senderMapping[userID.String()]
			if ok {
				sender := spec.SenderIDFromPseudoIDKey(roomKey)
				return &sender, nil
			} else {
				return nil, nil
			}
		} else {
			senderID := spec.SenderIDFromUserID(userID)
			return &senderID, nil
		}
	}

	return nil, fmt.Errorf("room not found")
}

func (s *sendEventTestRoomserverAPI) QueryUserIDForSender(
	ctx context.Context,
	roomID spec.RoomID,
	senderID spec.SenderID,
) (*spec.UserID, error) {
	if roomID.String() == s.roomIDStr {
		if s.roomVersion == gomatrixserverlib.RoomVersionPseudoIDs {
			for uID, roomKey := range s.senderMapping {
				if string(spec.SenderIDFromPseudoIDKey(roomKey)) == string(senderID) {
					parsedUserID, err := spec.NewUserID(uID, true)
					if err != nil {
						s.t.Fatalf("Mock QueryUserIDForSender failed: %s", err)
					}
					return parsedUserID, nil
				}
			}
		} else {
			userID := senderID.ToUserID()
			if userID == nil {
				return nil, fmt.Errorf("bad sender ID")
			}
			return userID, nil
		}
	}

	return nil, fmt.Errorf("room not found")
}

func (s *sendEventTestRoomserverAPI) SigningIdentityFor(ctx context.Context, roomID spec.RoomID, sender spec.UserID) (fclient.SigningIdentity, error) {
	if s.roomIDStr == roomID.String() {
		if s.roomVersion == gomatrixserverlib.RoomVersionPseudoIDs {
			roomKey, ok := s.senderMapping[sender.String()]
			if !ok {
				s.t.Logf("SigningIdentityFor used with unknown user ID: %v", sender.String())
				return fclient.SigningIdentity{}, fmt.Errorf("could not get signing identity for %v", sender.String())
			}
			return fclient.SigningIdentity{PrivateKey: roomKey}, nil
		} else {
			return fclient.SigningIdentity{PrivateKey: ed25519.NewKeyFromSeed(make([]byte, 32))}, nil
		}
	}

	return fclient.SigningIdentity{}, fmt.Errorf("room not found")
}

func (s *sendEventTestRoomserverAPI) InputRoomEvents(ctx context.Context, req *rsapi.InputRoomEventsRequest, res *rsapi.InputRoomEventsResponse) {
	s.savedInputRoomEvents = req.InputRoomEvents
}

// Test that user ID state keys are translated correctly
func Test_SendEvent_PseudoIDStateKeys(t *testing.T) {
	nonpseudoIDRoomVersion := gomatrixserverlib.RoomVersionV10
	pseudoIDRoomVersion := gomatrixserverlib.RoomVersionPseudoIDs

	senderKeySeed := make([]byte, 32)
	senderUserID := "@testuser:domain"
	senderPrivKey := ed25519.NewKeyFromSeed(senderKeySeed)
	senderPseudoID := string(spec.SenderIDFromPseudoIDKey(senderPrivKey))

	eventType := "com.example.test"
	roomIDStr := "!id:domain"

	device := &uapi.Device{
		UserID: senderUserID,
	}

	t.Run("user ID state key are not translated to room key in non-pseudo ID room", func(t *testing.T) {
		eventsJSON := []string{
			fmt.Sprintf(`{"type":"m.room.create","state_key":"","room_id":"%v","sender":"%v","content":{"creator":"%v","room_version":"%v"}}`, roomIDStr, senderUserID, senderUserID, nonpseudoIDRoomVersion),
			fmt.Sprintf(`{"type":"m.room.member","state_key":"%v","room_id":"%v","sender":"%v","content":{"membership":"join"}}`, senderUserID, roomIDStr, senderUserID),
		}

		roomState, err := createEvents(eventsJSON, nonpseudoIDRoomVersion)
		if err != nil {
			t.Fatalf("failed to prepare state events: %s", err.Error())
		}

		rsAPI := &sendEventTestRoomserverAPI{
			t:           t,
			roomIDStr:   roomIDStr,
			roomVersion: nonpseudoIDRoomVersion,
			roomState:   roomState,
		}

		req, err := http.NewRequest("POST", "https://domain", io.NopCloser(strings.NewReader("{}")))
		if err != nil {
			t.Fatalf("failed to make new request: %s", err.Error())
		}

		cfg := &config.ClientAPI{}

		resp := SendEvent(req, device, roomIDStr, eventType, nil, &senderUserID, cfg, rsAPI, nil)

		if resp.Code != http.StatusOK {
			t.Fatalf("non-200 HTTP code returned: %v\nfull response: %v", resp.Code, resp)
		}

		assert.Equal(t, len(rsAPI.savedInputRoomEvents), 1)

		ev := rsAPI.savedInputRoomEvents[0]
		stateKey := ev.Event.StateKey()
		if stateKey == nil {
			t.Fatalf("submitted InputRoomEvent has nil state key, when it should be %v", senderUserID)
		}
		if *stateKey != senderUserID {
			t.Fatalf("expected submitted InputRoomEvent to have user ID state key\nfound: %v\nexpected: %v", *stateKey, senderUserID)
		}
	})

	t.Run("user ID state key are translated to room key in pseudo ID room", func(t *testing.T) {
		eventsJSON := []string{
			fmt.Sprintf(`{"type":"m.room.create","state_key":"","room_id":"%v","sender":"%v","content":{"creator":"%v","room_version":"%v"}}`, roomIDStr, senderPseudoID, senderPseudoID, pseudoIDRoomVersion),
			fmt.Sprintf(`{"type":"m.room.member","state_key":"%v","room_id":"%v","sender":"%v","content":{"membership":"join"}}`, senderPseudoID, roomIDStr, senderPseudoID),
		}

		roomState, err := createEvents(eventsJSON, pseudoIDRoomVersion)
		if err != nil {
			t.Fatalf("failed to prepare state events: %s", err.Error())
		}

		rsAPI := &sendEventTestRoomserverAPI{
			t:           t,
			roomIDStr:   roomIDStr,
			roomVersion: pseudoIDRoomVersion,
			senderMapping: map[string]ed25519.PrivateKey{
				senderUserID: senderPrivKey,
			},
			roomState: roomState,
		}

		req, err := http.NewRequest("POST", "https://domain", io.NopCloser(strings.NewReader("{}")))
		if err != nil {
			t.Fatalf("failed to make new request: %s", err.Error())
		}

		cfg := &config.ClientAPI{}

		resp := SendEvent(req, device, roomIDStr, eventType, nil, &senderUserID, cfg, rsAPI, nil)

		if resp.Code != http.StatusOK {
			t.Fatalf("non-200 HTTP code returned: %v\nfull response: %v", resp.Code, resp)
		}

		assert.Equal(t, len(rsAPI.savedInputRoomEvents), 1)

		ev := rsAPI.savedInputRoomEvents[0]
		stateKey := ev.Event.StateKey()
		if stateKey == nil {
			t.Fatalf("submitted InputRoomEvent has nil state key, when it should be %v", senderPseudoID)
		}
		if *stateKey != senderPseudoID {
			t.Fatalf("expected submitted InputRoomEvent to have pseudo ID state key\nfound: %v\nexpected: %v", *stateKey, senderPseudoID)
		}
	})
}

func createEvents(eventsJSON []string, roomVer gomatrixserverlib.RoomVersion) ([]*types.HeaderedEvent, error) {
	events := make([]*types.HeaderedEvent, len(eventsJSON))

	roomVerImpl, err := gomatrixserverlib.GetRoomVersion(roomVer)
	if err != nil {
		return nil, fmt.Errorf("no roomver impl: %s", err.Error())
	}

	for i, eventJSON := range eventsJSON {
		pdu, evErr := roomVerImpl.NewEventFromTrustedJSON([]byte(eventJSON), false)
		if evErr != nil {
			return nil, fmt.Errorf("failed to make event: %s", evErr.Error())
		}
		ev := types.HeaderedEvent{PDU: pdu}
		events[i] = &ev
	}

	return events, nil
}
