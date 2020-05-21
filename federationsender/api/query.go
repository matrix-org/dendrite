package api

import (
	"context"

	internalHTTP "github.com/matrix-org/dendrite/internal/http"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/opentracing/opentracing-go"
)

// FederationSenderQueryJoinedHostsInRoomPath is the HTTP path for the QueryJoinedHostsInRoom API.
const FederationSenderQueryJoinedHostsInRoomPath = "/api/federationsender/queryJoinedHostsInRoom"

// FederationSenderQueryJoinedHostServerNamesInRoomPath is the HTTP path for the QueryJoinedHostServerNamesInRoom API.
const FederationSenderQueryJoinedHostServerNamesInRoomPath = "/api/federationsender/queryJoinedHostServerNamesInRoom"

// QueryJoinedHostsInRoomRequest is a request to QueryJoinedHostsInRoom
type QueryJoinedHostsInRoomRequest struct {
	RoomID string `json:"room_id"`
}

// QueryJoinedHostsInRoomResponse is a response to QueryJoinedHostsInRoom
type QueryJoinedHostsInRoomResponse struct {
	JoinedHosts []types.JoinedHost `json:"joined_hosts"`
}

// QueryJoinedHostsInRoom implements FederationSenderInternalAPI
func (h *httpFederationSenderInternalAPI) QueryJoinedHostsInRoom(
	ctx context.Context,
	request *QueryJoinedHostsInRoomRequest,
	response *QueryJoinedHostsInRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryJoinedHostsInRoom")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderQueryJoinedHostsInRoomPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryJoinedHostServerNamesRequest is a request to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomRequest struct {
	RoomID string `json:"room_id"`
}

// QueryJoinedHostServerNamesResponse is a response to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomResponse struct {
	ServerNames []gomatrixserverlib.ServerName `json:"server_names"`
}

// QueryJoinedHostServerNamesInRoom implements FederationSenderInternalAPI
func (h *httpFederationSenderInternalAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *QueryJoinedHostServerNamesInRoomRequest,
	response *QueryJoinedHostServerNamesInRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryJoinedHostServerNamesInRoom")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderQueryJoinedHostServerNamesInRoomPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
