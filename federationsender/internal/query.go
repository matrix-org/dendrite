package internal

import (
	"context"

	"github.com/matrix-org/dendrite/federationsender/api"
)

// QueryJoinedHostServerNamesInRoom implements api.FederationSenderInternalAPI
func (f *FederationSenderInternalAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostServerNamesInRoomRequest,
	response *api.QueryJoinedHostServerNamesInRoomResponse,
) (err error) {
	joinedHosts, err := f.db.GetJoinedHostsForRooms(ctx, []string{request.RoomID})
	if err != nil {
		return
	}
	response.ServerNames = joinedHosts

	return
}
