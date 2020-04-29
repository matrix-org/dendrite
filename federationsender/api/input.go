package api

import (
	"context"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	"github.com/opentracing/opentracing-go"
)

const (
	FederationSenderInputJoinRequestPath  = "/api/federationsender/inputJoinRequest"
	FederationSenderInputLeaveRequestPath = "/api/federationsender/inputLeaveRequest"
)

type InputJoinRequest struct {
	RoomID string `json:"room_id"`
}

type InputJoinResponse struct {
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpFederationSenderQueryAPI) InputJoinRequest(
	ctx context.Context,
	request *InputJoinRequest,
	response *InputJoinResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputJoinRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderInputJoinRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

type InputLeaveRequest struct {
	RoomID string `json:"room_id"`
}

type InputLeaveResponse struct {
}

// Handle an instruction to make_leave & send_leave with a remote server.
func (h *httpFederationSenderQueryAPI) InputLeaveRequest(
	ctx context.Context,
	request *InputLeaveRequest,
	response *InputLeaveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputLeaveRequest")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderInputLeaveRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
