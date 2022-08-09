package inthttp

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// AddRoutes adds the FederationInternalAPI handlers to the http.ServeMux.
// nolint:gocyclo
func AddRoutes(intAPI api.FederationInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(
		FederationAPIQueryJoinedHostServerNamesInRoomPath,
		httputil.MakeInternalRPCAPI("FederationAPIQueryJoinedHostServerNamesInRoom", intAPI.QueryJoinedHostServerNamesInRoom),
	)

	internalAPIMux.Handle(
		FederationAPIPerformInviteRequestPath,
		httputil.MakeInternalRPCAPI("FederationAPIPerformInvite", intAPI.PerformInvite),
	)

	internalAPIMux.Handle(
		FederationAPIPerformLeaveRequestPath,
		httputil.MakeInternalRPCAPI("FederationAPIPerformLeave", intAPI.PerformLeave),
	)

	internalAPIMux.Handle(
		FederationAPIPerformDirectoryLookupRequestPath,
		httputil.MakeInternalRPCAPI("FederationAPIPerformDirectoryLookupRequest", intAPI.PerformDirectoryLookup),
	)

	internalAPIMux.Handle(
		FederationAPIPerformBroadcastEDUPath,
		httputil.MakeInternalRPCAPI("FederationAPIPerformBroadcastEDU", intAPI.PerformBroadcastEDU),
	)

	internalAPIMux.Handle(
		FederationAPIPerformJoinRequestPath,
		httputil.MakeInternalRPCAPI(
			"FederationAPIPerformJoinRequest",
			func(ctx context.Context, req *api.PerformJoinRequest, res *api.PerformJoinResponse) error {
				intAPI.PerformJoin(ctx, req, res)
				return nil
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIGetUserDevicesPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPIGetUserDevices",
			func(ctx context.Context, req *getUserDevices) (*gomatrixserverlib.RespUserDevices, error) {
				res, err := intAPI.GetUserDevices(ctx, req.S, req.UserID)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIClaimKeysPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPIClaimKeys",
			func(ctx context.Context, req *claimKeys) (*gomatrixserverlib.RespClaimKeys, error) {
				res, err := intAPI.ClaimKeys(ctx, req.S, req.OneTimeKeys)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIQueryKeysPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPIQueryKeys",
			func(ctx context.Context, req *queryKeys) (*gomatrixserverlib.RespQueryKeys, error) {
				res, err := intAPI.QueryKeys(ctx, req.S, req.Keys)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIBackfillPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPIBackfill",
			func(ctx context.Context, req *backfill) (*gomatrixserverlib.Transaction, error) {
				res, err := intAPI.Backfill(ctx, req.S, req.RoomID, req.Limit, req.EventIDs)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPILookupStatePath,
		httputil.MakeInternalProxyAPI(
			"FederationAPILookupState",
			func(ctx context.Context, req *lookupState) (*gomatrixserverlib.RespState, error) {
				res, err := intAPI.LookupState(ctx, req.S, req.RoomID, req.EventID, req.RoomVersion)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPILookupStateIDsPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPILookupStateIDs",
			func(ctx context.Context, req *lookupStateIDs) (*gomatrixserverlib.RespStateIDs, error) {
				res, err := intAPI.LookupStateIDs(ctx, req.S, req.RoomID, req.EventID)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPILookupMissingEventsPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPILookupMissingEvents",
			func(ctx context.Context, req *lookupMissingEvents) (*gomatrixserverlib.RespMissingEvents, error) {
				res, err := intAPI.LookupMissingEvents(ctx, req.S, req.RoomID, req.Missing, req.RoomVersion)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIGetEventPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPIGetEvent",
			func(ctx context.Context, req *getEvent) (*gomatrixserverlib.Transaction, error) {
				res, err := intAPI.GetEvent(ctx, req.S, req.EventID)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIGetEventAuthPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPIGetEventAuth",
			func(ctx context.Context, req *getEventAuth) (*gomatrixserverlib.RespEventAuth, error) {
				res, err := intAPI.GetEventAuth(ctx, req.S, req.RoomVersion, req.RoomID, req.EventID)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIQueryServerKeysPath,
		httputil.MakeInternalRPCAPI("FederationAPIQueryServerKeys", intAPI.QueryServerKeys),
	)

	internalAPIMux.Handle(
		FederationAPILookupServerKeysPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPILookupServerKeys",
			func(ctx context.Context, req *lookupServerKeys) (*[]gomatrixserverlib.ServerKeys, error) {
				res, err := intAPI.LookupServerKeys(ctx, req.S, req.KeyRequests)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPIEventRelationshipsPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPIMSC2836EventRelationships",
			func(ctx context.Context, req *eventRelationships) (*gomatrixserverlib.MSC2836EventRelationshipsResponse, error) {
				res, err := intAPI.MSC2836EventRelationships(ctx, req.S, req.Req, req.RoomVer)
				return &res, federationClientError(err)
			},
		),
	)

	internalAPIMux.Handle(
		FederationAPISpacesSummaryPath,
		httputil.MakeInternalProxyAPI(
			"FederationAPIMSC2946SpacesSummary",
			func(ctx context.Context, req *spacesReq) (*gomatrixserverlib.MSC2946SpacesResponse, error) {
				res, err := intAPI.MSC2946Spaces(ctx, req.S, req.RoomID, req.SuggestedOnly)
				return &res, federationClientError(err)
			},
		),
	)

	// TODO: Look at this shape
	internalAPIMux.Handle(FederationAPIQueryPublicKeyPath,
		httputil.MakeInternalAPI("FederationAPIQueryPublicKeys", func(req *http.Request) util.JSONResponse {
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
		httputil.MakeInternalAPI("FederationAPIInputPublicKeys", func(req *http.Request) util.JSONResponse {
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
	switch ferr := err.(type) {
	case api.FederationClientError:
		return &ferr
	case *api.FederationClientError:
		return ferr
	default:
		return &api.FederationClientError{
			Err: err.Error(),
		}
	}
}
