package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
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

func (a *FederationSenderInternalAPI) QueryServerKeys(
	ctx context.Context, req *api.QueryServerKeysRequest, res *api.QueryServerKeysResponse,
) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	ires, err := a.doRequest(req.ServerName, func() (interface{}, error) {
		return a.federation.GetServerKeys(ctx, req.ServerName)
	})
	if err != nil {
		// try to load from the cache
		serverKeysResponses, dbErr := a.db.GetNotaryKeys(ctx, req.ServerName, req.OptionalKeyIDs)
		if dbErr != nil {
			return fmt.Errorf("server returned %s, and db returned %s", err, dbErr)
		}
		res.ServerKeys = serverKeysResponses
		return nil
	}
	serverKeys := ires.(gomatrixserverlib.ServerKeys)
	// cache it!
	if err = a.db.UpdateNotaryKeys(context.Background(), req.ServerName, serverKeys); err != nil {
		// non-fatal, still return the response
		util.GetLogger(ctx).WithError(err).Warn("failed to UpdateNotaryKeys")
	}
	res.ServerKeys = []gomatrixserverlib.ServerKeys{serverKeys}
	return nil
}
