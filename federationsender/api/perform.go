package api

import (
	"context"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	"github.com/opentracing/opentracing-go"
)

const (
	FederationSenderPerformJoinRequestPath  = "/api/federationsender/performJoinRequest"
	FederationSenderPerformLeaveRequestPath = "/api/federationsender/performLeaveRequest"
)

type PerformJoinRequest struct {
	RoomID string `json:"room_id"`
}

type PerformJoinResponse struct {
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationSenderInternalAPI) PerformJoinRequest(
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
func (h *httpFederationSenderInternalAPI) PerformLeaveRequest(
	ctx context.Context,
	request *PerformLeaveRequest,
	response *PerformLeaveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLeaveRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderPerformLeaveRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
