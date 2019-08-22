package query

import (
	"context"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// FederationSenderQueryDatabase has the APIs needed to implement the query API.
type FederationSenderQueryDatabase interface {
	GetJoinedHosts(
		ctx context.Context, roomID string,
	) ([]types.JoinedHost, error)
}

// FederationSenderQueryAPI is an implementation of api.FederationSenderQueryAPI
type FederationSenderQueryAPI struct {
	DB FederationSenderQueryDatabase
}

// QueryJoinedHostsInRoom implements api.FederationSenderQueryAPI
func (f *FederationSenderQueryAPI) QueryJoinedHostsInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostsInRoomRequest,
	response *api.QueryJoinedHostsInRoomResponse,
) (err error) {
	response.JoinedHosts, err = f.DB.GetJoinedHosts(ctx, request.RoomID)
	return
}

// QueryJoinedHostServerNamesInRoom implements api.FederationSenderQueryAPI
func (f *FederationSenderQueryAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostServerNamesInRoomRequest,
	response *api.QueryJoinedHostServerNamesInRoomResponse,
) (err error) {
	joinedHosts, err := f.DB.GetJoinedHosts(ctx, request.RoomID)
	if err != nil {
		return
	}

	serverNamesSet := make(map[gomatrixserverlib.ServerName]bool, len(joinedHosts))
	for _, host := range joinedHosts {
		serverNamesSet[host.ServerName] = true
	}

	response.ServerNames = make([]gomatrixserverlib.ServerName, 0, len(serverNamesSet))
	for name := range serverNamesSet {
		response.ServerNames = append(response.ServerNames, name)
	}

	return
}
