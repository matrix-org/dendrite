package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/gomatrix"
	"github.com/opentracing/opentracing-go"
)

// HTTP paths for the internal HTTP API
const (
	FederationSenderQueryJoinedHostServerNamesInRoomPath = "/federationsender/queryJoinedHostServerNamesInRoom"

	FederationSenderPerformDirectoryLookupRequestPath = "/federationsender/performDirectoryLookup"
	FederationSenderPerformJoinRequestPath            = "/federationsender/performJoinRequest"
	FederationSenderPerformLeaveRequestPath           = "/federationsender/performLeaveRequest"
	FederationSenderPerformServersAlivePath           = "/federationsender/performServersAlive"
	FederationSenderPerformBroadcastEDUPath           = "/federationsender/performBroadcastEDU"
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
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpFederationSenderInternalAPI) PerformServersAlive(
	ctx context.Context,
	request *api.PerformServersAliveRequest,
	response *api.PerformServersAliveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformServersAlive")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformServersAlivePath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
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
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationSenderInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformJoinRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformJoinRequestPath
	err := httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err != nil {
		response.LastError = &gomatrix.HTTPError{
			Message:      err.Error(),
			Code:         0,
			WrappedError: err,
		}
	}
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
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// Handle an instruction to broadcast an EDU to all servers in rooms we are joined to.
func (h *httpFederationSenderInternalAPI) PerformBroadcastEDU(
	ctx context.Context,
	request *api.PerformBroadcastEDURequest,
	response *api.PerformBroadcastEDUResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformBroadcastEDU")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformBroadcastEDUPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
