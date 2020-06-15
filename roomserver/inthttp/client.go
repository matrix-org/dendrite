package inthttp

import (
	"context"
	"errors"
	"net/http"

	fsInputAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
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
	RoomserverPerformJoinPath     = "/roomserver/performJoin"
	RoomserverPerformLeavePath    = "/roomserver/performLeave"
	RoomserverPerformBackfillPath = "/roomserver/performBackfill"

	// Query operations
	RoomserverQueryLatestEventsAndStatePath    = "/roomserver/queryLatestEventsAndState"
	RoomserverQueryStateAfterEventsPath        = "/roomserver/queryStateAfterEvents"
	RoomserverQueryEventsByIDPath              = "/roomserver/queryEventsByID"
	RoomserverQueryMembershipForUserPath       = "/roomserver/queryMembershipForUser"
	RoomserverQueryMembershipsForRoomPath      = "/roomserver/queryMembershipsForRoom"
	RoomserverQueryServerAllowedToSeeEventPath = "/roomserver/queryServerAllowedToSeeEvent"
	RoomserverQueryMissingEventsPath           = "/roomserver/queryMissingEvents"
	RoomserverQueryStateAndAuthChainPath       = "/roomserver/queryStateAndAuthChain"
	RoomserverQueryRoomVersionCapabilitiesPath = "/roomserver/queryRoomVersionCapabilities"
	RoomserverQueryRoomVersionForRoomPath      = "/roomserver/queryRoomVersionForRoom"
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

// SetFederationSenderInputAPI no-ops in HTTP client mode as there is no chicken/egg scenario
func (h *httpRoomserverInternalAPI) SetFederationSenderAPI(fsAPI fsInputAPI.FederationSenderInternalAPI) {
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
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputRoomEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverInputRoomEventsPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpRoomserverInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformJoin")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformJoinPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
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
