package inthttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/util"
)

// AddRoutes adds the FederationSenderInternalAPI handlers to the http.ServeMux.
// nolint:gocyclo
func AddRoutes(intAPI api.FederationSenderInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(
		FederationSenderQueryJoinedHostServerNamesInRoomPath,
		httputil.MakeInternalAPI("QueryJoinedHostServerNamesInRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryJoinedHostServerNamesInRoomRequest
			var response api.QueryJoinedHostServerNamesInRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := intAPI.QueryJoinedHostServerNamesInRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderPerformJoinRequestPath,
		httputil.MakeInternalAPI("PerformJoinRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformJoinRequest
			var response api.PerformJoinResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			intAPI.PerformJoin(req.Context(), &request, &response)
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderPerformLeaveRequestPath,
		httputil.MakeInternalAPI("PerformLeaveRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformLeaveRequest
			var response api.PerformLeaveResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformLeave(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderPerformInviteRequestPath,
		httputil.MakeInternalAPI("PerformInviteRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformInviteRequest
			var response api.PerformInviteResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformInvite(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderPerformDirectoryLookupRequestPath,
		httputil.MakeInternalAPI("PerformDirectoryLookupRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformDirectoryLookupRequest
			var response api.PerformDirectoryLookupResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformDirectoryLookup(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderPerformServersAlivePath,
		httputil.MakeInternalAPI("PerformServersAliveRequest", func(req *http.Request) util.JSONResponse {
			var request api.PerformServersAliveRequest
			var response api.PerformServersAliveResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformServersAlive(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderPerformBroadcastEDUPath,
		httputil.MakeInternalAPI("PerformBroadcastEDU", func(req *http.Request) util.JSONResponse {
			var request api.PerformBroadcastEDURequest
			var response api.PerformBroadcastEDUResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.PerformBroadcastEDU(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderGetUserDevicesPath,
		httputil.MakeInternalAPI("GetUserDevices", func(req *http.Request) util.JSONResponse {
			var request getUserDevices
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.GetUserDevices(req.Context(), request.S, request.UserID)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.Res = &res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderClaimKeysPath,
		httputil.MakeInternalAPI("ClaimKeys", func(req *http.Request) util.JSONResponse {
			var request claimKeys
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.ClaimKeys(req.Context(), request.S, request.OneTimeKeys)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.Res = &res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderQueryKeysPath,
		httputil.MakeInternalAPI("QueryKeys", func(req *http.Request) util.JSONResponse {
			var request queryKeys
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.QueryKeys(req.Context(), request.S, request.Keys)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.Res = &res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderBackfillPath,
		httputil.MakeInternalAPI("Backfill", func(req *http.Request) util.JSONResponse {
			var request backfill
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.Backfill(req.Context(), request.S, request.RoomID, request.Limit, request.EventIDs)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.Res = &res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderLookupStatePath,
		httputil.MakeInternalAPI("LookupState", func(req *http.Request) util.JSONResponse {
			var request lookupState
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.LookupState(req.Context(), request.S, request.RoomID, request.EventID, request.RoomVersion)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.Res = &res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderLookupStateIDsPath,
		httputil.MakeInternalAPI("LookupStateIDs", func(req *http.Request) util.JSONResponse {
			var request lookupStateIDs
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.LookupStateIDs(req.Context(), request.S, request.RoomID, request.EventID)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.Res = &res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderGetEventPath,
		httputil.MakeInternalAPI("GetEvent", func(req *http.Request) util.JSONResponse {
			var request getEvent
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.GetEvent(req.Context(), request.S, request.EventID)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.Res = &res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderGetServerKeysPath,
		httputil.MakeInternalAPI("GetServerKeys", func(req *http.Request) util.JSONResponse {
			var request getServerKeys
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.GetServerKeys(req.Context(), request.S)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.ServerKeys = res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderLookupServerKeysPath,
		httputil.MakeInternalAPI("LookupServerKeys", func(req *http.Request) util.JSONResponse {
			var request lookupServerKeys
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.LookupServerKeys(req.Context(), request.S, request.KeyRequests)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.ServerKeys = res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderEventRelationshipsPath,
		httputil.MakeInternalAPI("MSC2836EventRelationships", func(req *http.Request) util.JSONResponse {
			var request eventRelationships
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.MSC2836EventRelationships(req.Context(), request.S, request.Req, request.RoomVer)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.Res = res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationSenderSpacesSummaryPath,
		httputil.MakeInternalAPI("MSC2946SpacesSummary", func(req *http.Request) util.JSONResponse {
			var request spacesReq
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.MSC2946Spaces(req.Context(), request.S, request.RoomID, request.Req)
			if err != nil {
				ferr, ok := err.(*api.FederationClientError)
				if ok {
					request.Err = ferr
				} else {
					request.Err = &api.FederationClientError{
						Err: err.Error(),
					}
				}
			}
			request.Res = res
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
}
