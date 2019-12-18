package query

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
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

	response.ServerNames = make([]gomatrixserverlib.ServerName, 0, len(joinedHosts))
	for _, host := range joinedHosts {
		response.ServerNames = append(response.ServerNames, host.ServerName)
	}

	return
}

// SetupHTTP adds the FederationSenderQueryAPI handlers to the http.ServeMux.
func (f *FederationSenderQueryAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(
		api.FederationSenderQueryJoinedHostsInRoomPath,
		common.MakeInternalAPI("QueryJoinedHostsInRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryJoinedHostsInRoomRequest
			var response api.QueryJoinedHostsInRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := f.QueryJoinedHostsInRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.FederationSenderQueryJoinedHostServerNamesInRoomPath,
		common.MakeInternalAPI("QueryJoinedHostServerNamesInRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryJoinedHostServerNamesInRoomRequest
			var response api.QueryJoinedHostServerNamesInRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := f.QueryJoinedHostServerNamesInRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
