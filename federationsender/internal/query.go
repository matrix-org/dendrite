package internal

import (
	"context"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// QueryJoinedHostServerNamesInRoom implements api.FederationSenderInternalAPI
func (f *FederationSenderInternalAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostServerNamesInRoomRequest,
	response *api.QueryJoinedHostServerNamesInRoomResponse,
) (err error) {
	joinedHosts, err := f.db.GetJoinedHosts(ctx, request.RoomID)
	if err != nil {
		return
	}

	response.ServerNames = make([]gomatrixserverlib.ServerName, 0, len(joinedHosts))
	for _, host := range joinedHosts {
		response.ServerNames = append(response.ServerNames, host.ServerName)
	}

	// TODO: remove duplicates?

	return
}
