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
	roomQuery roomserver.QueryEventsAPI
}

func NewStore(roomQuery roomserver.QueryEventsAPI) Store {
	return Store{
		roomQuery: roomQuery,
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
	err := s.roomQuery.QueryCurrentState(context.Background(), &roomserver.QueryCurrentStateRequest{
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
	doneSearching := false
	for _, state := range roomEvents.StateEvents {
		switch state.Type() {
		case gomatrixserverlib.MRoomCreate:
			// Space is created with no children yet.
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
			// Space is created and has one or more children.
			result.RoomType = Space
			result.SpaceNetworkId = roomId
			doneSearching = true
		case ConstSpaceParentEventType:
			// Channel is created and has a reference to the parent.
			result.RoomType = Channel
			result.SpaceNetworkId = *state.StateKey()
			result.ChannelNetworkId = roomId
			doneSearching = true
		}
		if doneSearching {
			break
		}
	}

	return result
}
