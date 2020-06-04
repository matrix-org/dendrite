package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/federationsender/api"
	internalHTTP "github.com/matrix-org/dendrite/internal/http"
	"github.com/opentracing/opentracing-go"
)

// HTTP paths for the internal HTTP API
const (
	FederationSenderQueryJoinedHostsInRoomPath           = "/federationsender/queryJoinedHostsInRoom"
	FederationSenderQueryJoinedHostServerNamesInRoomPath = "/federationsender/queryJoinedHostServerNamesInRoom"

	FederationSenderPerformDirectoryLookupRequestPath = "/federationsender/performDirectoryLookup"
	FederationSenderPerformJoinRequestPath            = "/federationsender/performJoinRequest"
	FederationSenderPerformLeaveRequestPath           = "/federationsender/performLeaveRequest"
	FederationSenderPerformServersAlivePath           = "/federationsender/performServersAlive"
)

// NewFederationSenderClient creates a FederationSenderInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewFederationSenderClient(federationSenderURL string, httpClient *http.Client) (api.FederationSenderInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewFederationSenderInternalAPIHTTP: httpClient is <nil>")
	}
	return &httpFederationSenderInternalAPI{federationSenderURL, httpClient}, nil
}

type httpFederationSenderInternalAPI struct {
	federationSenderURL string
	httpClient          *http.Client
}

// Handle an instruction to make_leave & send_leave with a remote server.
func (h *httpFederationSenderInternalAPI) PerformLeave(
	ctx context.Context,
	request *api.PerformLeaveRequest,
	response *api.PerformLeaveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLeaveRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformLeaveRequestPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpFederationSenderInternalAPI) PerformServersAlive(
	ctx context.Context,
	request *api.PerformServersAliveRequest,
	response *api.PerformServersAliveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformServersAlive")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformServersAlivePath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryJoinedHostServerNamesInRoom implements FederationSenderInternalAPI
func (h *httpFederationSenderInternalAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostServerNamesInRoomRequest,
	response *api.QueryJoinedHostServerNamesInRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryJoinedHostServerNamesInRoom")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderQueryJoinedHostServerNamesInRoomPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryJoinedHostsInRoom implements FederationSenderInternalAPI
func (h *httpFederationSenderInternalAPI) QueryJoinedHostsInRoom(
	ctx context.Context,
	request *api.QueryJoinedHostsInRoomRequest,
	response *api.QueryJoinedHostsInRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryJoinedHostsInRoom")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderQueryJoinedHostsInRoomPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationSenderInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformJoinRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformJoinRequestPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationSenderInternalAPI) PerformDirectoryLookup(
	ctx context.Context,
	request *api.PerformDirectoryLookupRequest,
	response *api.PerformDirectoryLookupResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformDirectoryLookup")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformDirectoryLookupRequestPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
