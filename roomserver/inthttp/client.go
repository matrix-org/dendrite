package inthttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	asAPI "github.com/matrix-org/dendrite/appservice/api"
	fsInputAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/opentracing/opentracing-go"
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
	RoomserverPerformInvitePath      = "/roomserver/performInvite"
	RoomserverPerformPeekPath        = "/roomserver/performPeek"
	RoomserverPerformUnpeekPath      = "/roomserver/performUnpeek"
	RoomserverPerformJoinPath        = "/roomserver/performJoin"
	RoomserverPerformLeavePath       = "/roomserver/performLeave"
	RoomserverPerformBackfillPath    = "/roomserver/performBackfill"
	RoomserverPerformPublishPath     = "/roomserver/performPublish"
	RoomserverPerformInboundPeekPath = "/roomserver/performInboundPeek"
	RoomserverPerformForgetPath      = "/roomserver/performForget"

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
func (h *httpRoomserverInternalAPI) SetFederationAPI(fsAPI fsInputAPI.FederationInternalAPI, keyRing *gomatrixserverlib.KeyRing) {
}

// SetAppserviceAPI no-ops in HTTP client mode as there is no chicken/egg scenario
func (h *httpRoomserverInternalAPI) SetAppserviceAPI(asAPI asAPI.AppServiceQueryAPI) {
}

// SetRoomAlias implements RoomserverAliasAPI
func (h *httpRoomserverInternalAPI) SetRoomAlias(
	ctx context.Context,
	request *api.SetRoomAliasRequest,
	response *api.SetRoomAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "SetRoomAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverSetRoomAliasPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetRoomIDForAlias implements RoomserverAliasAPI
func (h *httpRoomserverInternalAPI) GetRoomIDForAlias(
	ctx context.Context,
	request *api.GetRoomIDForAliasRequest,
	response *api.GetRoomIDForAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetRoomIDForAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverGetRoomIDForAliasPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetAliasesForRoomID implements RoomserverAliasAPI
func (h *httpRoomserverInternalAPI) GetAliasesForRoomID(
	ctx context.Context,
	request *api.GetAliasesForRoomIDRequest,
	response *api.GetAliasesForRoomIDResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetAliasesForRoomID")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverGetAliasesForRoomIDPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// GetCreatorIDForAlias implements RoomserverAliasAPI
func (h *httpRoomserverInternalAPI) GetCreatorIDForAlias(
	ctx context.Context,
	request *api.GetCreatorIDForAliasRequest,
	response *api.GetCreatorIDForAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "GetCreatorIDForAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverGetCreatorIDForAliasPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// RemoveRoomAlias implements RoomserverAliasAPI
func (h *httpRoomserverInternalAPI) RemoveRoomAlias(
	ctx context.Context,
	request *api.RemoveRoomAliasRequest,
	response *api.RemoveRoomAliasResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "RemoveRoomAlias")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverRemoveRoomAliasPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// InputRoomEvents implements RoomserverInputAPI
func (h *httpRoomserverInternalAPI) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputRoomEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverInputRoomEventsPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err != nil {
		response.ErrMsg = err.Error()
	}
}

func (h *httpRoomserverInternalAPI) PerformInvite(
	ctx context.Context,
	request *api.PerformInviteRequest,
	response *api.PerformInviteResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformInvite")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformInvitePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpRoomserverInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformJoin")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformJoinPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err != nil {
		response.Error = &api.PerformError{
			Msg: fmt.Sprintf("failed to communicate with roomserver: %s", err),
		}
	}
}

func (h *httpRoomserverInternalAPI) PerformPeek(
	ctx context.Context,
	request *api.PerformPeekRequest,
	response *api.PerformPeekResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformPeek")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformPeekPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err != nil {
		response.Error = &api.PerformError{
			Msg: fmt.Sprintf("failed to communicate with roomserver: %s", err),
		}
	}
}

func (h *httpRoomserverInternalAPI) PerformInboundPeek(
	ctx context.Context,
	request *api.PerformInboundPeekRequest,
	response *api.PerformInboundPeekResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformInboundPeek")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformInboundPeekPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpRoomserverInternalAPI) PerformUnpeek(
	ctx context.Context,
	request *api.PerformUnpeekRequest,
	response *api.PerformUnpeekResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformUnpeek")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformUnpeekPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err != nil {
		response.Error = &api.PerformError{
			Msg: fmt.Sprintf("failed to communicate with roomserver: %s", err),
		}
	}
}

func (h *httpRoomserverInternalAPI) PerformLeave(
	ctx context.Context,
	request *api.PerformLeaveRequest,
	response *api.PerformLeaveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLeave")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformLeavePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpRoomserverInternalAPI) PerformPublish(
	ctx context.Context,
	req *api.PerformPublishRequest,
	res *api.PerformPublishResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformPublish")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformPublishPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
	if err != nil {
		res.Error = &api.PerformError{
			Msg: fmt.Sprintf("failed to communicate with roomserver: %s", err),
		}
	}
}

// QueryLatestEventsAndState implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryLatestEventsAndState(
	ctx context.Context,
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryLatestEventsAndState")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryLatestEventsAndStatePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryStateAfterEvents implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryStateAfterEvents(
	ctx context.Context,
	request *api.QueryStateAfterEventsRequest,
	response *api.QueryStateAfterEventsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryStateAfterEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryStateAfterEventsPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryEventsByID implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryEventsByID(
	ctx context.Context,
	request *api.QueryEventsByIDRequest,
	response *api.QueryEventsByIDResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryEventsByID")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryEventsByIDPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpRoomserverInternalAPI) QueryPublishedRooms(
	ctx context.Context,
	request *api.QueryPublishedRoomsRequest,
	response *api.QueryPublishedRoomsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryPublishedRooms")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryPublishedRoomsPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryMembershipForUser implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryMembershipForUser(
	ctx context.Context,
	request *api.QueryMembershipForUserRequest,
	response *api.QueryMembershipForUserResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryMembershipForUser")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryMembershipForUserPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryMembershipsForRoom implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryMembershipsForRoom(
	ctx context.Context,
	request *api.QueryMembershipsForRoomRequest,
	response *api.QueryMembershipsForRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryMembershipsForRoom")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryMembershipsForRoomPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryMembershipsForRoom implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryServerJoinedToRoom(
	ctx context.Context,
	request *api.QueryServerJoinedToRoomRequest,
	response *api.QueryServerJoinedToRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryServerJoinedToRoom")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryServerJoinedToRoomPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryServerAllowedToSeeEvent implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryServerAllowedToSeeEvent(
	ctx context.Context,
	request *api.QueryServerAllowedToSeeEventRequest,
	response *api.QueryServerAllowedToSeeEventResponse,
) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryServerAllowedToSeeEvent")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryServerAllowedToSeeEventPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryMissingEvents implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) QueryMissingEvents(
	ctx context.Context,
	request *api.QueryMissingEventsRequest,
	response *api.QueryMissingEventsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryMissingEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryMissingEventsPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryStateAndAuthChain implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryStateAndAuthChain(
	ctx context.Context,
	request *api.QueryStateAndAuthChainRequest,
	response *api.QueryStateAndAuthChainResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryStateAndAuthChain")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryStateAndAuthChainPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// PerformBackfill implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) PerformBackfill(
	ctx context.Context,
	request *api.PerformBackfillRequest,
	response *api.PerformBackfillResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformBackfill")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformBackfillPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryRoomVersionCapabilities implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) QueryRoomVersionCapabilities(
	ctx context.Context,
	request *api.QueryRoomVersionCapabilitiesRequest,
	response *api.QueryRoomVersionCapabilitiesResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryRoomVersionCapabilities")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryRoomVersionCapabilitiesPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
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

	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryRoomVersionForRoom")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryRoomVersionForRoomPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
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
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryCurrentState")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryCurrentStatePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpRoomserverInternalAPI) QueryRoomsForUser(
	ctx context.Context,
	request *api.QueryRoomsForUserRequest,
	response *api.QueryRoomsForUserResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryRoomsForUser")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryRoomsForUserPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpRoomserverInternalAPI) QueryBulkStateContent(
	ctx context.Context,
	request *api.QueryBulkStateContentRequest,
	response *api.QueryBulkStateContentResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryBulkStateContent")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryBulkStateContentPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpRoomserverInternalAPI) QuerySharedUsers(
	ctx context.Context, req *api.QuerySharedUsersRequest, res *api.QuerySharedUsersResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QuerySharedUsers")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQuerySharedUsersPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpRoomserverInternalAPI) QueryKnownUsers(
	ctx context.Context, req *api.QueryKnownUsersRequest, res *api.QueryKnownUsersResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryKnownUsers")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryKnownUsersPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpRoomserverInternalAPI) QueryAuthChain(
	ctx context.Context, req *api.QueryAuthChainRequest, res *api.QueryAuthChainResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryAuthChain")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryAuthChainPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpRoomserverInternalAPI) QueryServerBannedFromRoom(
	ctx context.Context, req *api.QueryServerBannedFromRoomRequest, res *api.QueryServerBannedFromRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryServerBannedFromRoom")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryServerBannedFromRoomPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpRoomserverInternalAPI) PerformForget(ctx context.Context, req *api.PerformForgetRequest, res *api.PerformForgetResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformForget")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformForgetPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)

}
