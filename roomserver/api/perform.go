package api

import (
	"context"

	internalHTTP "github.com/matrix-org/dendrite/internal/http"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/opentracing/opentracing-go"
)

const (
	// RoomserverPerformJoinPath is the HTTP path for the PerformJoin API.
	RoomserverPerformJoinPath = "/api/roomserver/performJoin"

	// RoomserverPerformLeavePath is the HTTP path for the PerformLeave API.
	RoomserverPerformLeavePath = "/api/roomserver/performLeave"
)

type PerformJoinRequest struct {
	RoomIDOrAlias string                         `json:"room_id_or_alias"`
	UserID        string                         `json:"user_id"`
	Content       map[string]interface{}         `json:"content"`
	ServerNames   []gomatrixserverlib.ServerName `json:"server_names"`
}

type PerformJoinResponse struct {
}

func (h *httpRoomserverInternalAPI) PerformJoin(
	ctx context.Context,
	request *PerformJoinRequest,
	response *PerformJoinResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformJoin")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformJoinPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

type PerformLeaveRequest struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
}

type PerformLeaveResponse struct {
}

func (h *httpRoomserverInternalAPI) PerformLeave(
	ctx context.Context,
	request *PerformLeaveRequest,
	response *PerformLeaveResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformLeave")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverPerformLeavePath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
