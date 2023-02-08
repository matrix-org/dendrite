package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/gomatrixserverlib"

	asAPI "github.com/matrix-org/dendrite/appservice/api"
	fsInputAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

const (
	// Alias operations
	RoomserverSetRoomAliasPath         = "/roomserver/setRoomAlias"
	RoomserverGetRoomIDForAliasPath    = "/roomserver/GetRoomIDForAlias"
	RoomserverGetAliasesForRoomIDPath  = "/roomserver/GetAliasesForRoomID"
	RoomserverGetCreatorIDForAliasPath = "/roomserver/GetCreatorIDForAlias"
	RoomserverRemoveRoomAliasPath      = "/roomserver/removeRoomAlias"

	// Input operations
	RoomserverInputRoomEventsPath = "/roomserver/inputRoomEvents"

	// Perform operations
	RoomserverPerformInvitePath             = "/roomserver/performInvite"
	RoomserverPerformPeekPath               = "/roomserver/performPeek"
	RoomserverPerformUnpeekPath             = "/roomserver/performUnpeek"
	RoomserverPerformRoomUpgradePath        = "/roomserver/performRoomUpgrade"
	RoomserverPerformJoinPath               = "/roomserver/performJoin"
	RoomserverPerformLeavePath              = "/roomserver/performLeave"
	RoomserverPerformBackfillPath           = "/roomserver/performBackfill"
	RoomserverPerformPublishPath            = "/roomserver/performPublish"
	RoomserverPerformInboundPeekPath        = "/roomserver/performInboundPeek"
	RoomserverPerformForgetPath             = "/roomserver/performForget"
	RoomserverPerformAdminEvacuateRoomPath  = "/roomserver/performAdminEvacuateRoom"
	RoomserverPerformAdminEvacuateUserPath  = "/roomserver/performAdminEvacuateUser"
	RoomserverPerformAdminDownloadStatePath = "/roomserver/performAdminDownloadState"

	// Query operations
	RoomserverQueryLatestEventsAndStatePath    = "/roomserver/queryLatestEventsAndState"
	RoomserverQueryStateAfterEventsPath        = "/roomserver/queryStateAfterEvents"
	RoomserverQueryEventsByIDPath              = "/roomserver/queryEventsByID"
	RoomserverQueryMembershipForUserPath       = "/roomserver/queryMembershipForUser"
	RoomserverQueryMembershipsForRoomPath      = "/roomserver/queryMembershipsForRoom"
	RoomserverQueryServerJoinedToRoomPath      = "/roomserver/queryServerJoinedToRoomPath"
	RoomserverQueryServerAllowedToSeeEventPath = "/roomserver/queryServerAllowedToSeeEvent"
	RoomserverQueryMissingEventsPath           = "/roomserver/queryMissingEvents"
	RoomserverQueryStateAndAuthChainPath       = "/roomserver/queryStateAndAuthChain"
	RoomserverQueryRoomVersionCapabilitiesPath = "/roomserver/queryRoomVersionCapabilities"
	RoomserverQueryRoomVersionForRoomPath      = "/roomserver/queryRoomVersionForRoom"
	RoomserverQueryPublishedRoomsPath          = "/roomserver/queryPublishedRooms"
	RoomserverQueryCurrentStatePath            = "/roomserver/queryCurrentState"
	RoomserverQueryRoomsForUserPath            = "/roomserver/queryRoomsForUser"
	RoomserverQueryBulkStateContentPath        = "/roomserver/queryBulkStateContent"
	RoomserverQuerySharedUsersPath             = "/roomserver/querySharedUsers"
	RoomserverQueryKnownUsersPath              = "/roomserver/queryKnownUsers"
	RoomserverQueryServerBannedFromRoomPath    = "/roomserver/queryServerBannedFromRoom"
	RoomserverQueryAuthChainPath               = "/roomserver/queryAuthChain"
	RoomserverQueryRestrictedJoinAllowed       = "/roomserver/queryRestrictedJoinAllowed"
	RoomserverQueryMembershipAtEventPath       = "/roomserver/queryMembershipAtEvent"
)

type httpRoomserverInternalAPI struct {
	roomserverURL string
	httpClient    *http.Client
	cache         caching.RoomVersionCache
}

// NewRoomserverClient creates a RoomserverInputAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewRoomserverClient(
	roomserverURL string,
	httpClient *http.Client,
	cache caching.RoomVersionCache,
) (api.RoomserverInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewRoomserverInternalAPIHTTP: httpClient is <nil>")
	}
	return &httpRoomserverInternalAPI{
		roomserverURL: roomserverURL,
		httpClient:    httpClient,
		cache:         cache,
	}, nil
}

// SetFederationInputAPI no-ops in HTTP client mode as there is no chicken/egg scenario
func (h *httpRoomserverInternalAPI) SetFederationAPI(fsAPI fsInputAPI.RoomserverFederationAPI, keyRing *gomatrixserverlib.KeyRing) {
}

// SetAppserviceAPI no-ops in HTTP client mode as there is no chicken/egg scenario
func (h *httpRoomserverInternalAPI) SetAppserviceAPI(asAPI asAPI.AppServiceInternalAPI) {
}

// SetUserAPI no-ops in HTTP client mode as there is no chicken/egg scenario
func (h *httpRoomserverInternalAPI) SetUserAPI(userAPI userapi.RoomserverUserAPI) {
}

// SetRoomAlias implements RoomserverAliasAPI
func (h *httpRoomserverInternalAPI) SetRoomAlias(
	ctx context.Context,
	request *api.SetRoomAliasRequest,
	response *api.SetRoomAliasResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"SetRoomAlias", h.roomserverURL+RoomserverSetRoomAliasPath,
		h.httpClient, ctx, request, response,
	)
}

// GetRoomIDForAlias implements RoomserverAliasAPI
func (h *httpRoomserverInternalAPI) GetRoomIDForAlias(
	ctx context.Context,
	request *api.GetRoomIDForAliasRequest,
	response *api.GetRoomIDForAliasResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"GetRoomIDForAlias", h.roomserverURL+RoomserverGetRoomIDForAliasPath,
		h.httpClient, ctx, request, response,
	)
}

// GetAliasesForRoomID implements RoomserverAliasAPI
func (h *httpRoomserverInternalAPI) GetAliasesForRoomID(
	ctx context.Context,
	request *api.GetAliasesForRoomIDRequest,
	response *api.GetAliasesForRoomIDResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"GetAliasesForRoomID", h.roomserverURL+RoomserverGetAliasesForRoomIDPath,
		h.httpClient, ctx, request, response,
	)
}

// RemoveRoomAlias implements RoomserverAliasAPI
func (h *httpRoomserverInternalAPI) RemoveRoomAlias(
	ctx context.Context,
	request *api.RemoveRoomAliasRequest,
	response *api.RemoveRoomAliasResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"RemoveRoomAlias", h.roomserverURL+RoomserverRemoveRoomAliasPath,
		h.httpClient, ctx, request, response,
	)
}

// InputRoomEvents implements RoomserverInputAPI
func (h *httpRoomserverInternalAPI) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) error {
	if err := httputil.CallInternalRPCAPI(
		"InputRoomEvents", h.roomserverURL+RoomserverInputRoomEventsPath,
		h.httpClient, ctx, request, response,
	); err != nil {
		response.ErrMsg = err.Error()
	}
	return nil
}

func (h *httpRoomserverInternalAPI) PerformInvite(
	ctx context.Context,
	request *api.PerformInviteRequest,
	response *api.PerformInviteResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformInvite", h.roomserverURL+RoomserverPerformInvitePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformJoin", h.roomserverURL+RoomserverPerformJoinPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformPeek(
	ctx context.Context,
	request *api.PerformPeekRequest,
	response *api.PerformPeekResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformPeek", h.roomserverURL+RoomserverPerformPeekPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformInboundPeek(
	ctx context.Context,
	request *api.PerformInboundPeekRequest,
	response *api.PerformInboundPeekResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformInboundPeek", h.roomserverURL+RoomserverPerformInboundPeekPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformUnpeek(
	ctx context.Context,
	request *api.PerformUnpeekRequest,
	response *api.PerformUnpeekResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformUnpeek", h.roomserverURL+RoomserverPerformUnpeekPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformRoomUpgrade(
	ctx context.Context,
	request *api.PerformRoomUpgradeRequest,
	response *api.PerformRoomUpgradeResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformRoomUpgrade", h.roomserverURL+RoomserverPerformRoomUpgradePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformLeave(
	ctx context.Context,
	request *api.PerformLeaveRequest,
	response *api.PerformLeaveResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformLeave", h.roomserverURL+RoomserverPerformLeavePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformPublish(
	ctx context.Context,
	request *api.PerformPublishRequest,
	response *api.PerformPublishResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformPublish", h.roomserverURL+RoomserverPerformPublishPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformAdminEvacuateRoom(
	ctx context.Context,
	request *api.PerformAdminEvacuateRoomRequest,
	response *api.PerformAdminEvacuateRoomResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformAdminEvacuateRoom", h.roomserverURL+RoomserverPerformAdminEvacuateRoomPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformAdminDownloadState(
	ctx context.Context,
	request *api.PerformAdminDownloadStateRequest,
	response *api.PerformAdminDownloadStateResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformAdminDownloadState", h.roomserverURL+RoomserverPerformAdminDownloadStatePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformAdminEvacuateUser(
	ctx context.Context,
	request *api.PerformAdminEvacuateUserRequest,
	response *api.PerformAdminEvacuateUserResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformAdminEvacuateUser", h.roomserverURL+RoomserverPerformAdminEvacuateUserPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryLatestEventsAndState implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryLatestEventsAndState(
	ctx context.Context,
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryLatestEventsAndState", h.roomserverURL+RoomserverQueryLatestEventsAndStatePath,
		h.httpClient, ctx, request, response,
	)
}

// QueryStateAfterEvents implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryStateAfterEvents(
	ctx context.Context,
	request *api.QueryStateAfterEventsRequest,
	response *api.QueryStateAfterEventsResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryStateAfterEvents", h.roomserverURL+RoomserverQueryStateAfterEventsPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryEventsByID implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryEventsByID(
	ctx context.Context,
	request *api.QueryEventsByIDRequest,
	response *api.QueryEventsByIDResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryEventsByID", h.roomserverURL+RoomserverQueryEventsByIDPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) QueryPublishedRooms(
	ctx context.Context,
	request *api.QueryPublishedRoomsRequest,
	response *api.QueryPublishedRoomsResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryPublishedRooms", h.roomserverURL+RoomserverQueryPublishedRoomsPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryMembershipForUser implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryMembershipForUser(
	ctx context.Context,
	request *api.QueryMembershipForUserRequest,
	response *api.QueryMembershipForUserResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryMembershipForUser", h.roomserverURL+RoomserverQueryMembershipForUserPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryMembershipsForRoom implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryMembershipsForRoom(
	ctx context.Context,
	request *api.QueryMembershipsForRoomRequest,
	response *api.QueryMembershipsForRoomResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryMembershipsForRoom", h.roomserverURL+RoomserverQueryMembershipsForRoomPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryMembershipsForRoom implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryServerJoinedToRoom(
	ctx context.Context,
	request *api.QueryServerJoinedToRoomRequest,
	response *api.QueryServerJoinedToRoomResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryServerJoinedToRoom", h.roomserverURL+RoomserverQueryServerJoinedToRoomPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryServerAllowedToSeeEvent implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryServerAllowedToSeeEvent(
	ctx context.Context,
	request *api.QueryServerAllowedToSeeEventRequest,
	response *api.QueryServerAllowedToSeeEventResponse,
) (err error) {
	return httputil.CallInternalRPCAPI(
		"QueryServerAllowedToSeeEvent", h.roomserverURL+RoomserverQueryServerAllowedToSeeEventPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryMissingEvents implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) QueryMissingEvents(
	ctx context.Context,
	request *api.QueryMissingEventsRequest,
	response *api.QueryMissingEventsResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryMissingEvents", h.roomserverURL+RoomserverQueryMissingEventsPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryStateAndAuthChain implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryStateAndAuthChain(
	ctx context.Context,
	request *api.QueryStateAndAuthChainRequest,
	response *api.QueryStateAndAuthChainResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryStateAndAuthChain", h.roomserverURL+RoomserverQueryStateAndAuthChainPath,
		h.httpClient, ctx, request, response,
	)
}

// PerformBackfill implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) PerformBackfill(
	ctx context.Context,
	request *api.PerformBackfillRequest,
	response *api.PerformBackfillResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformBackfill", h.roomserverURL+RoomserverPerformBackfillPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryRoomVersionCapabilities implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) QueryRoomVersionCapabilities(
	ctx context.Context,
	request *api.QueryRoomVersionCapabilitiesRequest,
	response *api.QueryRoomVersionCapabilitiesResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryRoomVersionCapabilities", h.roomserverURL+RoomserverQueryRoomVersionCapabilitiesPath,
		h.httpClient, ctx, request, response,
	)
}

// QueryRoomVersionForRoom implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) QueryRoomVersionForRoom(
	ctx context.Context,
	request *api.QueryRoomVersionForRoomRequest,
	response *api.QueryRoomVersionForRoomResponse,
) error {
	if roomVersion, ok := h.cache.GetRoomVersion(request.RoomID); ok {
		response.RoomVersion = roomVersion
		return nil
	}
	err := httputil.CallInternalRPCAPI(
		"QueryRoomVersionForRoom", h.roomserverURL+RoomserverQueryRoomVersionForRoomPath,
		h.httpClient, ctx, request, response,
	)
	if err == nil {
		h.cache.StoreRoomVersion(request.RoomID, response.RoomVersion)
	}
	return err
}

func (h *httpRoomserverInternalAPI) QueryCurrentState(
	ctx context.Context,
	request *api.QueryCurrentStateRequest,
	response *api.QueryCurrentStateResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryCurrentState", h.roomserverURL+RoomserverQueryCurrentStatePath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) QueryRoomsForUser(
	ctx context.Context,
	request *api.QueryRoomsForUserRequest,
	response *api.QueryRoomsForUserResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryRoomsForUser", h.roomserverURL+RoomserverQueryRoomsForUserPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) QueryBulkStateContent(
	ctx context.Context,
	request *api.QueryBulkStateContentRequest,
	response *api.QueryBulkStateContentResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryBulkStateContent", h.roomserverURL+RoomserverQueryBulkStateContentPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) QuerySharedUsers(
	ctx context.Context,
	request *api.QuerySharedUsersRequest,
	response *api.QuerySharedUsersResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QuerySharedUsers", h.roomserverURL+RoomserverQuerySharedUsersPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) QueryKnownUsers(
	ctx context.Context,
	request *api.QueryKnownUsersRequest,
	response *api.QueryKnownUsersResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryKnownUsers", h.roomserverURL+RoomserverQueryKnownUsersPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) QueryAuthChain(
	ctx context.Context,
	request *api.QueryAuthChainRequest,
	response *api.QueryAuthChainResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryAuthChain", h.roomserverURL+RoomserverQueryAuthChainPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) QueryServerBannedFromRoom(
	ctx context.Context,
	request *api.QueryServerBannedFromRoomRequest,
	response *api.QueryServerBannedFromRoomResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryServerBannedFromRoom", h.roomserverURL+RoomserverQueryServerBannedFromRoomPath,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) QueryRestrictedJoinAllowed(
	ctx context.Context,
	request *api.QueryRestrictedJoinAllowedRequest,
	response *api.QueryRestrictedJoinAllowedResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"QueryRestrictedJoinAllowed", h.roomserverURL+RoomserverQueryRestrictedJoinAllowed,
		h.httpClient, ctx, request, response,
	)
}

func (h *httpRoomserverInternalAPI) PerformForget(
	ctx context.Context,
	request *api.PerformForgetRequest,
	response *api.PerformForgetResponse,
) error {
	return httputil.CallInternalRPCAPI(
		"PerformForget", h.roomserverURL+RoomserverPerformForgetPath,
		h.httpClient, ctx, request, response,
	)

}

func (h *httpRoomserverInternalAPI) QueryMembershipAtEvent(ctx context.Context, request *api.QueryMembershipAtEventRequest, response *api.QueryMembershipAtEventResponse) error {
	return httputil.CallInternalRPCAPI(
		"QueryMembershiptAtEvent", h.roomserverURL+RoomserverQueryMembershipAtEventPath,
		h.httpClient, ctx, request, response,
	)
}
