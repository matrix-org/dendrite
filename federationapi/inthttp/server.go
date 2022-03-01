package inthttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/util"
)

// AddRoutes adds the FederationInternalAPI handlers to the http.ServeMux.
// nolint:gocyclo
func AddRoutes(intAPI api.FederationInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(
		FederationAPIQueryJoinedHostServerNamesInRoomPath,
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
		FederationAPIPerformJoinRequestPath,
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
		FederationAPIPerformLeaveRequestPath,
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
		FederationAPIPerformInviteRequestPath,
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
		FederationAPIPerformDirectoryLookupRequestPath,
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
		FederationAPIPerformServersAlivePath,
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
		FederationAPIPerformBroadcastEDUPath,
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
		FederationAPIGetUserDevicesPath,
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
		FederationAPIClaimKeysPath,
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
		FederationAPIQueryKeysPath,
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
		FederationAPIBackfillPath,
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
		FederationAPILookupStatePath,
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
		FederationAPILookupStateIDsPath,
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
		FederationAPILookupMissingEventsPath,
		httputil.MakeInternalAPI("LookupMissingEvents", func(req *http.Request) util.JSONResponse {
			var request lookupMissingEvents
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.LookupMissingEvents(req.Context(), request.S, request.RoomID, request.Missing, request.RoomVersion)
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
			for _, event := range res.Events {
				js, err := json.Marshal(event)
				if err != nil {
					return util.MessageResponse(http.StatusInternalServerError, err.Error())
				}
				request.Res.Events = append(request.Res.Events, js)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: request}
		}),
	)
	internalAPIMux.Handle(
		FederationAPIGetEventPath,
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
		FederationAPIGetEventAuthPath,
		httputil.MakeInternalAPI("GetEventAuth", func(req *http.Request) util.JSONResponse {
			var request getEventAuth
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.GetEventAuth(req.Context(), request.S, request.RoomVersion, request.RoomID, request.EventID)
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
		FederationAPIQueryServerKeysPath,
		httputil.MakeInternalAPI("QueryServerKeys", func(req *http.Request) util.JSONResponse {
			var request api.QueryServerKeysRequest
			var response api.QueryServerKeysResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.QueryServerKeys(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(
		FederationAPILookupServerKeysPath,
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
		FederationAPIEventRelationshipsPath,
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
		FederationAPISpacesSummaryPath,
		httputil.MakeInternalAPI("MSC2946SpacesSummary", func(req *http.Request) util.JSONResponse {
			var request spacesReq
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			res, err := intAPI.MSC2946Spaces(req.Context(), request.S, request.RoomID, request.SuggestedOnly)
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
	internalAPIMux.Handle(FederationAPIQueryPublicKeyPath,
		httputil.MakeInternalAPI("queryPublicKeys", func(req *http.Request) util.JSONResponse {
			request := api.QueryPublicKeysRequest{}
			response := api.QueryPublicKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			keys, err := intAPI.FetchKeys(req.Context(), request.Requests)
			if err != nil {
				return util.ErrorResponse(err)
			}
			response.Results = keys
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(FederationAPIInputPublicKeyPath,
		httputil.MakeInternalAPI("inputPublicKeys", func(req *http.Request) util.JSONResponse {
			request := api.InputPublicKeysRequest{}
			response := api.InputPublicKeysResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := intAPI.StoreKeys(req.Context(), request.Keys); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
