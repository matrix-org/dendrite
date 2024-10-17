package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	rsapi "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	uapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"gotest.tools/v3/assert"
)

var ()

type stateTestRoomserverAPI struct {
	rsapi.RoomserverInternalAPI
	t           *testing.T
	roomState   map[gomatrixserverlib.StateKeyTuple]*types.HeaderedEvent
	roomIDStr   string
	roomVersion gomatrixserverlib.RoomVersion
	userIDStr   string
	// userID -> senderID
	senderMapping map[string]string
}

func (s stateTestRoomserverAPI) QueryRoomVersionForRoom(ctx context.Context, roomID string) (gomatrixserverlib.RoomVersion, error) {
	if roomID == s.roomIDStr {
		return s.roomVersion, nil
	} else {
		s.t.Logf("room version queried for %s", roomID)
		return "", fmt.Errorf("unknown room")
	}
}

func (s stateTestRoomserverAPI) QueryLatestEventsAndState(
	ctx context.Context,
	req *rsapi.QueryLatestEventsAndStateRequest,
	res *rsapi.QueryLatestEventsAndStateResponse,
) error {
	res.RoomExists = req.RoomID == s.roomIDStr
	if !res.RoomExists {
		return nil
	}

	res.StateEvents = []*types.HeaderedEvent{}
	for _, stateKeyTuple := range req.StateToFetch {
		val, ok := s.roomState[stateKeyTuple]
		if ok && val != nil {
			res.StateEvents = append(res.StateEvents, val)
		}
	}

	return nil
}

func (s stateTestRoomserverAPI) QueryMembershipForUser(
	ctx context.Context,
	req *rsapi.QueryMembershipForUserRequest,
	res *rsapi.QueryMembershipForUserResponse,
) error {
	if req.UserID.String() == s.userIDStr {
		res.HasBeenInRoom = true
		res.IsInRoom = true
		res.RoomExists = true
		res.Membership = spec.Join
	}

	return nil
}

func (s stateTestRoomserverAPI) QuerySenderIDForUser(
	ctx context.Context,
	roomID spec.RoomID,
	userID spec.UserID,
) (*spec.SenderID, error) {
	sID, ok := s.senderMapping[userID.String()]
	if ok {
		sender := spec.SenderID(sID)
		return &sender, nil
	} else {
		return nil, nil
	}
}

func (s stateTestRoomserverAPI) QueryUserIDForSender(
	ctx context.Context,
	roomID spec.RoomID,
	senderID spec.SenderID,
) (*spec.UserID, error) {
	for uID, sID := range s.senderMapping {
		if sID == string(senderID) {
			parsedUserID, err := spec.NewUserID(uID, true)
			if err != nil {
				s.t.Fatalf("Mock QueryUserIDForSender failed: %s", err)
			}
			return parsedUserID, nil
		}
	}
	return nil, nil
}

func (s stateTestRoomserverAPI) QueryStateAfterEvents(
	ctx context.Context,
	req *rsapi.QueryStateAfterEventsRequest,
	res *rsapi.QueryStateAfterEventsResponse,
) error {
	return nil
}

func Test_OnIncomingStateTypeRequest(t *testing.T) {
	var tempRoomServerCfg config.RoomServer
	tempRoomServerCfg.Defaults(config.DefaultOpts{})
	defaultRoomVersion := tempRoomServerCfg.DefaultRoomVersion
	pseudoIDRoomVersion := gomatrixserverlib.RoomVersionPseudoIDs
	nonPseudoIDRoomVersion := gomatrixserverlib.RoomVersionV10

	userIDStr := "@testuser:domain"
	eventType := "com.example.test"
	stateKey := "testStateKey"
	roomIDStr := "!id:domain"

	device := &uapi.Device{
		UserID: userIDStr,
	}

	t.Run("request simple state key", func(t *testing.T) {
		ctx := context.Background()

		rsAPI := stateTestRoomserverAPI{
			roomVersion: defaultRoomVersion,
			roomIDStr:   roomIDStr,
			roomState: map[gomatrixserverlib.StateKeyTuple]*types.HeaderedEvent{
				{
					EventType: eventType,
					StateKey:  stateKey,
				}: mustCreateStatePDU(t, defaultRoomVersion, roomIDStr, eventType, stateKey, map[string]interface{}{
					"foo": "bar",
				}),
			},
			userIDStr: userIDStr,
		}

		jsonResp := OnIncomingStateTypeRequest(ctx, device, rsAPI, roomIDStr, eventType, stateKey, false)

		assert.DeepEqual(t, jsonResp, util.JSONResponse{
			Code: http.StatusOK,
			JSON: spec.RawJSON(`{"foo":"bar"}`),
		})
	})

	t.Run("user ID key translated to room key in pseudo ID rooms", func(t *testing.T) {
		ctx := context.Background()

		stateSenderUserID := "@sender:domain"
		stateSenderRoomKey := "testsenderkey"

		rsAPI := stateTestRoomserverAPI{
			roomVersion: pseudoIDRoomVersion,
			roomIDStr:   roomIDStr,
			roomState: map[gomatrixserverlib.StateKeyTuple]*types.HeaderedEvent{
				{
					EventType: eventType,
					StateKey:  stateSenderRoomKey,
				}: mustCreateStatePDU(t, pseudoIDRoomVersion, roomIDStr, eventType, stateSenderRoomKey, map[string]interface{}{
					"foo": "bar",
				}),
				{
					EventType: eventType,
					StateKey:  stateSenderUserID,
				}: mustCreateStatePDU(t, pseudoIDRoomVersion, roomIDStr, eventType, stateSenderUserID, map[string]interface{}{
					"not": "thisone",
				}),
			},
			userIDStr: userIDStr,
			senderMapping: map[string]string{
				stateSenderUserID: stateSenderRoomKey,
			},
		}

		jsonResp := OnIncomingStateTypeRequest(ctx, device, rsAPI, roomIDStr, eventType, stateSenderUserID, false)

		assert.DeepEqual(t, jsonResp, util.JSONResponse{
			Code: http.StatusOK,
			JSON: spec.RawJSON(`{"foo":"bar"}`),
		})
	})

	t.Run("user ID key not translated to room key in non-pseudo ID rooms", func(t *testing.T) {
		ctx := context.Background()

		stateSenderUserID := "@sender:domain"
		stateSenderRoomKey := "testsenderkey"

		rsAPI := stateTestRoomserverAPI{
			roomVersion: nonPseudoIDRoomVersion,
			roomIDStr:   roomIDStr,
			roomState: map[gomatrixserverlib.StateKeyTuple]*types.HeaderedEvent{
				{
					EventType: eventType,
					StateKey:  stateSenderRoomKey,
				}: mustCreateStatePDU(t, nonPseudoIDRoomVersion, roomIDStr, eventType, stateSenderRoomKey, map[string]interface{}{
					"not": "thisone",
				}),
				{
					EventType: eventType,
					StateKey:  stateSenderUserID,
				}: mustCreateStatePDU(t, nonPseudoIDRoomVersion, roomIDStr, eventType, stateSenderUserID, map[string]interface{}{
					"foo": "bar",
				}),
			},
			userIDStr: userIDStr,
			senderMapping: map[string]string{
				stateSenderUserID: stateSenderUserID,
			},
		}

		jsonResp := OnIncomingStateTypeRequest(ctx, device, rsAPI, roomIDStr, eventType, stateSenderUserID, false)

		assert.DeepEqual(t, jsonResp, util.JSONResponse{
			Code: http.StatusOK,
			JSON: spec.RawJSON(`{"foo":"bar"}`),
		})
	})
}

func mustCreateStatePDU(t *testing.T, roomVer gomatrixserverlib.RoomVersion, roomID string, stateType string, stateKey string, stateContent map[string]interface{}) *types.HeaderedEvent {
	t.Helper()
	roomVerImpl := gomatrixserverlib.MustGetRoomVersion(roomVer)

	evBytes, err := json.Marshal(map[string]interface{}{
		"room_id":   roomID,
		"type":      stateType,
		"state_key": stateKey,
		"content":   stateContent,
	})
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	ev, err := roomVerImpl.NewEventFromTrustedJSON(evBytes, false)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}

	return &types.HeaderedEvent{PDU: ev}
}
