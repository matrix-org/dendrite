package query

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/types"
	rsAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/util"
)

// FederationSenderQueryDatabase has the APIs needed to implement the query API.
type FederationSenderQueryDatabase interface {
	GetJoinedHosts(
		ctx context.Context, roomID string,
	) ([]types.JoinedHost, error)
}

// FederationSenderInternalAPI is an implementation of api.FederationSenderInternalAPI
type FederationSenderInternalAPI struct {
	api.FederationSenderInternalAPI
	DB                 FederationSenderQueryDatabase
	RoomserverInputAPI rsAPI.RoomserverInputAPI
}

// SetupHTTP adds the FederationSenderInternalAPI handlers to the http.ServeMux.
func (f *FederationSenderInternalAPI) SetupHTTP(servMux *http.ServeMux) {
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
	servMux.Handle(api.FederationSenderInputJoinRequestPath,
		common.MakeInternalAPI("inputJoinRequest", func(req *http.Request) util.JSONResponse {
			var request api.InputJoinRequest
			var response api.InputJoinResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := f.InputJoinRequest(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(api.FederationSenderInputLeaveRequestPath,
		common.MakeInternalAPI("inputLeaveRequest", func(req *http.Request) util.JSONResponse {
			var request api.InputLeaveRequest
			var response api.InputLeaveResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := f.InputLeaveRequest(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
