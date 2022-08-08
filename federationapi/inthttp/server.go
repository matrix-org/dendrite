package inthttp

import (
	"context"
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
		httputil.MakeInternalRPCAPI("QueryJoinedHostServerNamesInRoom", intAPI.QueryJoinedHostServerNamesInRoom),
	)

	internalAPIMux.Handle(
		FederationAPIPerformInviteRequestPath,
		httputil.MakeInternalRPCAPI("PerformInvite", intAPI.PerformInvite),
	)

	internalAPIMux.Handle(
		FederationAPIPerformLeaveRequestPath,
		httputil.MakeInternalRPCAPI("PerformLeave", intAPI.PerformLeave),
	)

	internalAPIMux.Handle(
		FederationAPIPerformDirectoryLookupRequestPath,
		httputil.MakeInternalRPCAPI("PerformDirectoryLookupRequest", intAPI.PerformDirectoryLookup),
	)

	internalAPIMux.Handle(
		FederationAPIPerformBroadcastEDUPath,
		httputil.MakeInternalRPCAPI("PerformJoinRequest", intAPI.PerformBroadcastEDU),
	)

	internalAPIMux.Handle(
		FederationAPIPerformJoinRequestPath,
		httputil.MakeInternalRPCAPI(
			"PerformJoinRequest",
			func(ctx context.Context, req *api.PerformJoinRequest, res *api.PerformJoinResponse) error {
				intAPI.PerformJoin(ctx, req, res)
				return nil
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIPerformJoinRequestPath,
		httputil.MakeInternalProxyAPI(
			"GetUserDevices",
			func(ctx context.Context, req *getUserDevices) {
				res, err := intAPI.GetUserDevices(ctx, req.S, req.UserID)
				req.Res, req.Err = &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIClaimKeysPath,
		httputil.MakeInternalProxyAPI(
			"ClaimKeys",
			func(ctx context.Context, req *claimKeys) {
				res, err := intAPI.ClaimKeys(ctx, req.S, req.OneTimeKeys)
				req.Res, req.Err = &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIQueryKeysPath,
		httputil.MakeInternalProxyAPI(
			"QueryKeys",
			func(ctx context.Context, req *queryKeys) {
				res, err := intAPI.QueryKeys(ctx, req.S, req.Keys)
				req.Res, req.Err = &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIBackfillPath,
		httputil.MakeInternalProxyAPI(
			"Backfill",
			func(ctx context.Context, req *backfill) {
				res, err := intAPI.Backfill(ctx, req.S, req.RoomID, req.Limit, req.EventIDs)
				req.Res, req.Err = &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPILookupStatePath,
		httputil.MakeInternalProxyAPI(
			"LookupState",
			func(ctx context.Context, req *lookupState) {
				res, err := intAPI.LookupState(ctx, req.S, req.RoomID, req.EventID, req.RoomVersion)
				req.Res, req.Err = &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPILookupStateIDsPath,
		httputil.MakeInternalProxyAPI(
			"LookupStateIDs",
			func(ctx context.Context, req *lookupState) {
				res, err := intAPI.LookupState(ctx, req.S, req.RoomID, req.EventID, req.RoomVersion)
				req.Res, req.Err = &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPILookupMissingEventsPath,
		httputil.MakeInternalProxyAPI(
			"LookupMissingEvents",
			func(ctx context.Context, req *lookupMissingEvents) {
				res, err := intAPI.LookupMissingEvents(ctx, req.S, req.RoomID, req.Missing, req.RoomVersion)
				for _, event := range res.Events {
					var js []byte
					js, err = json.Marshal(event)
					if err != nil {
						req.Err = federationClientError(err)
						return
					}
					req.Res.Events = append(req.Res.Events, js)
				}
				req.Err = federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIGetEventPath,
		httputil.MakeInternalProxyAPI(
			"GetEvent",
			func(ctx context.Context, req *getEvent) {
				res, err := intAPI.GetEvent(ctx, req.S, req.EventID)
				req.Res, req.Err = &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIGetEventAuthPath,
		httputil.MakeInternalProxyAPI(
			"GetEventAuth",
			func(ctx context.Context, req *getEventAuth) {
				res, err := intAPI.GetEventAuth(ctx, req.S, req.RoomVersion, req.RoomID, req.EventID)
				req.Res, req.Err = &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIQueryServerKeysPath,
		httputil.MakeInternalRPCAPI("QueryServerKeys", intAPI.QueryServerKeys),
	)

	internalAPIMux.Handle(
		FederationAPILookupServerKeysPath,
		httputil.MakeInternalProxyAPI(
			"LookupServerKeys",
			func(ctx context.Context, req *lookupServerKeys) {
				res, err := intAPI.LookupServerKeys(ctx, req.S, req.KeyRequests)
				req.ServerKeys, req.Err = res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIEventRelationshipsPath,
		httputil.MakeInternalProxyAPI(
			"MSC2836EventRelationships",
			func(ctx context.Context, req *eventRelationships) {
				res, err := intAPI.MSC2836EventRelationships(ctx, req.S, req.Req, req.RoomVer)
				req.Res, req.Err = res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPISpacesSummaryPath,
		httputil.MakeInternalProxyAPI(
			"MSC2946SpacesSummary",
			func(ctx context.Context, req *spacesReq) {
				res, err := intAPI.MSC2946Spaces(ctx, req.S, req.RoomID, req.SuggestedOnly)
				req.Res, req.Err = res, federationClientError(err)
			},
		),
	)

	// TODO: Look at this shape
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

	// TODO: Look at this shape
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

func federationClientError(err error) *api.FederationClientError {
	if err == nil {
		return nil
	}
	if ferr, ok := err.(*api.FederationClientError); ok {
		return ferr
	} else {
		return &api.FederationClientError{
			Err: err.Error(),
		}
	}
}
