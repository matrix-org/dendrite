package api

import (
	"context"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	"github.com/opentracing/opentracing-go"
)

const (
	// RoomserverPerformJoinRequestPath is the HTTP path for the PerformJoinRequest API.
	RoomserverPerformJoinRequestPath = "/api/roomserver/performJoin"

	// RoomserverPerformLeaveRequestPath is the HTTP path for the PerformLeaveRequest API.
	RoomserverPerformLeaveRequestPath = "/api/roomserver/performLeave"
)

type PerformJoinRequest struct {
	RoomID  string                 `json:"room_id"`
	UserID  string                 `json:"user_id"`
	Content map[string]interface{} `json:"content"`
}

type PerformJoinResponse struct {
}

// Handle an instruction to make_join & send_join with a remote server.
func (h *httpRoomserverInternalAPI) PerformJoin(
	ctx context.Context,
	request *PerformJoinRequest,
	response *PerformJoinResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformJoinRequest")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformJoinRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

type PerformLeaveRequest struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
}

type PerformLeaveResponse struct {
}

// Handle an instruction to make_leave & send_leave with a remote server.
func (h *httpRoomserverInternalAPI) PerformLeave(
	ctx context.Context,
	request *PerformLeaveRequest,
	response *PerformLeaveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLeaveRequest")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformLeaveRequestPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
