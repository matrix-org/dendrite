/*
Convenient function for space info mapping between Matrix room and Space contract
*/
package zion

import (
	"context"
	"encoding/json"
	"strings"

	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type ClientRoomserverStore struct {
	rsAPI roomserver.ClientRoomserverAPI
}

type SyncRoomserverStore struct {
	rsAPI roomserver.SyncRoomserverAPI
}

type StoreAPI interface {
	GetRoomInfo(roomId string, userId UserIdentifier) RoomInfo
}

func (s *ClientRoomserverStore) GetRoomInfo(roomId string, userId UserIdentifier) RoomInfo {
	result := RoomInfo{
		QueryUserId:      userId.MatrixUserId,
		SpaceNetworkId:   "",
		ChannelNetworkId: "",
		RoomType:         Unknown,
		IsOwner:          false,
	}

	createTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomCreate,
		StateKey:  "",
	}

	spaceChildTuple := gomatrixserverlib.StateKeyTuple{
		EventType: ConstSpaceChildEventType,
		StateKey:  "*",
	}

	spaceParentTuple := gomatrixserverlib.StateKeyTuple{
		EventType: ConstSpaceParentEventType,
		StateKey:  "*",
	}

	var roomEvents roomserver.QueryCurrentStateResponse
	err := s.rsAPI.QueryCurrentState(context.Background(), &roomserver.QueryCurrentStateRequest{
		RoomID:         roomId,
		AllowWildcards: true,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			createTuple,
			spaceParentTuple,
			spaceChildTuple,
		},
	}, &roomEvents)

	if err != nil {
		return result
	}
	//TODO: replace with HydrateRoomInfoWithStateEvents when you have a practical way to flatten roomEvents map
	for _, state := range roomEvents.StateEvents {
		switch state.Type() {
		case gomatrixserverlib.MRoomCreate:
			var creatorEvent CreatorEvent
			err := json.Unmarshal(roomEvents.StateEvents[createTuple].Content(), &creatorEvent)
			result.IsOwner = strings.HasPrefix(
				creatorEvent.Creator,
				userId.LocalPart,
			)
			if err == nil {
				result.RoomType = Space
				result.SpaceNetworkId = roomId
			}
		case ConstSpaceChildEventType:
			result.RoomType = Space
			result.SpaceNetworkId = roomId
		case ConstSpaceParentEventType:
			result.RoomType = Channel
			result.SpaceNetworkId = *state.StateKey()
			result.ChannelNetworkId = roomId
		}
	}

	return result
}

func (s *SyncRoomserverStore) GetRoomInfo(roomId string, userId UserIdentifier) RoomInfo {
	result := RoomInfo{
		QueryUserId:      userId.MatrixUserId,
		SpaceNetworkId:   "",
		ChannelNetworkId: "",
		RoomType:         Unknown,
		IsOwner:          false,
	}

	createTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomCreate,
		StateKey:  "",
	}

	spaceChildTuple := gomatrixserverlib.StateKeyTuple{
		EventType: ConstSpaceChildEventType,
		StateKey:  "*",
	}

	spaceParentTuple := gomatrixserverlib.StateKeyTuple{
		EventType: ConstSpaceParentEventType,
		StateKey:  "*",
	}

	var roomEvents roomserver.QueryLatestEventsAndStateResponse
	err := s.rsAPI.QueryLatestEventsAndState(context.Background(),
		&roomserver.QueryLatestEventsAndStateRequest{
			RoomID: roomId,
			StateToFetch: []gomatrixserverlib.StateKeyTuple{
				createTuple,
				spaceParentTuple,
				spaceChildTuple,
			},
		}, &roomEvents)

	if err != nil {
		return result
	}

	HydrateRoomInfoWithStateEvents(roomId, userId, &result, roomEvents.StateEvents)

	return result
}

func HydrateRoomInfoWithStateEvents(roomId string, userId UserIdentifier, r *RoomInfo, stateEvents []*gomatrixserverlib.HeaderedEvent) {
	for _, state := range stateEvents {
		switch state.Type() {
		case gomatrixserverlib.MRoomCreate:
			var creatorEvent CreatorEvent
			err := json.Unmarshal(state.Content(), &creatorEvent)
			r.IsOwner = strings.HasPrefix(
				creatorEvent.Creator,
				userId.LocalPart,
			)
			if err == nil {
				r.RoomType = Space
				r.SpaceNetworkId = roomId
			}
		case ConstSpaceChildEventType:
			r.RoomType = Space
			r.SpaceNetworkId = roomId
		case ConstSpaceParentEventType:
			r.RoomType = Channel
			r.SpaceNetworkId = *state.StateKey()
			r.ChannelNetworkId = roomId
		}
	}

}

func NewClientRoomserverStore(rsAPI roomserver.ClientRoomserverAPI) StoreAPI {
	return &ClientRoomserverStore{
		rsAPI: rsAPI,
	}
}

func NewSyncRoomserverStore(rsAPI roomserver.SyncRoomserverAPI) StoreAPI {
	return &SyncRoomserverStore{
		rsAPI: rsAPI,
	}
}
