package api

import (
	"context"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/opentracing/opentracing-go"
)

const (
	// FederationSenderPerformJoinRequestPath is the HTTP path for the PerformJoinRequest API.
	FederationSenderPerformDirectoryLookupRequestPath = "/api/federationsender/performDirectoryLookup"

	// FederationSenderPerformJoinRequestPath is the HTTP path for the PerformJoinRequest API.
	FederationSenderPerformJoinRequestPath = "/api/federationsender/performJoinRequest"

	// FederationSenderPerformLeaveRequestPath is the HTTP path for the PerformLeaveRequest API.
	FederationSenderPerformLeaveRequestPath = "/api/federationsender/performLeaveRequest"
)

type PerformDirectoryLookupRequest struct {
	RoomAlias  string                       `json:"room_alias"`
	ServerName gomatrixserverlib.ServerName `json:"server_name"`
}

type PerformDirectoryLookupResponse struct {
	RoomID      string                         `json:"room_id"`
	ServerNames []gomatrixserverlib.ServerName `json:"server_names"`
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationSenderInternalAPI) PerformDirectoryLookup(
	ctx context.Context,
	request *PerformDirectoryLookupRequest,
	response *PerformDirectoryLookupResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformDirectoryLookup")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformDirectoryLookupRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

type PerformJoinRequest struct {
	RoomID     string                       `json:"room_id"`
	UserID     string                       `json:"user_id"`
	ServerName gomatrixserverlib.ServerName `json:"server_name"`
	Content    map[string]interface{}       `json:"content"`
}

type PerformJoinResponse struct {
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationSenderInternalAPI) PerformJoin(
	ctx context.Context,
	request *PerformJoinRequest,
	response *PerformJoinResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformJoinRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformJoinRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

type PerformLeaveRequest struct {
	RoomID string `json:"room_id"`
}

type PerformLeaveResponse struct {
}

// Handle an instruction to make_leave & send_leave with a remote server.
func (h *httpFederationSenderInternalAPI) PerformLeave(
	ctx context.Context,
	request *PerformLeaveRequest,
	response *PerformLeaveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLeaveRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformLeaveRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
