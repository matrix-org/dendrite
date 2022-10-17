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

type Store struct {
	rsAPI roomserver.ClientRoomserverAPI
}

func NewStore(rsAPI roomserver.ClientRoomserverAPI) Store {
	return Store{
		rsAPI: rsAPI,
	}
}

func (s *Store) GetRoomInfo(roomId string, userId UserIdentifier) RoomInfo {
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
