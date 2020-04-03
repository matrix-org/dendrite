package api

import (
	"context"
	"errors"
	"net/http"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/opentracing/opentracing-go"
)

// QueryJoinedHostsInRoomRequest is a request to QueryJoinedHostsInRoom
type QueryJoinedHostsInRoomRequest struct {
	RoomID string `json:"room_id"`
}

// QueryJoinedHostsInRoomResponse is a response to QueryJoinedHostsInRoom
type QueryJoinedHostsInRoomResponse struct {
	JoinedHosts []types.JoinedHost `json:"joined_hosts"`
}

// QueryJoinedHostServerNamesRequest is a request to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomRequest struct {
	RoomID string `json:"room_id"`
}

// QueryJoinedHostServerNamesResponse is a response to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomResponse struct {
	ServerNames []gomatrixserverlib.ServerName `json:"server_names"`
}

// FederationSenderQueryAPI is used to query information from the federation sender.
type FederationSenderQueryAPI interface {
	// Query the joined hosts and the membership events accounting for their participation in a room.
	// Note that if a server has multiple users in the room, it will have multiple entries in the returned slice.
	// See `QueryJoinedHostServerNamesInRoom` for a de-duplicated version.
	QueryJoinedHostsInRoom(
		ctx context.Context,
		request *QueryJoinedHostsInRoomRequest,
		response *QueryJoinedHostsInRoomResponse,
	) error
	// Query the server names of the joined hosts in a room.
	// Unlike QueryJoinedHostsInRoom, this function returns a de-duplicated slice
	// containing only the server names (without information for membership events).
	QueryJoinedHostServerNamesInRoom(
		ctx context.Context,
		request *QueryJoinedHostServerNamesInRoomRequest,
		response *QueryJoinedHostServerNamesInRoomResponse,
	) error
}

// FederationSenderQueryJoinedHostsInRoomPath is the HTTP path for the QueryJoinedHostsInRoom API.
const FederationSenderQueryJoinedHostsInRoomPath = "/api/federationsender/queryJoinedHostsInRoom"

// FederationSenderQueryJoinedHostServerNamesInRoomPath is the HTTP path for the QueryJoinedHostServerNamesInRoom API.
const FederationSenderQueryJoinedHostServerNamesInRoomPath = "/api/federationsender/queryJoinedHostServerNamesInRoom"

// NewFederationSenderQueryAPIHTTP creates a FederationSenderQueryAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewFederationSenderQueryAPIHTTP(federationSenderURL string, httpClient *http.Client) (FederationSenderQueryAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewFederationSenderQueryAPIHTTP: httpClient is <nil>")
	}
	return &httpFederationSenderQueryAPI{federationSenderURL, httpClient}, nil
}

type httpFederationSenderQueryAPI struct {
	federationSenderURL string
	httpClient          *http.Client
}

// QueryJoinedHostsInRoom implements FederationSenderQueryAPI
func (h *httpFederationSenderQueryAPI) QueryJoinedHostsInRoom(
	ctx context.Context,
	request *QueryJoinedHostsInRoomRequest,
	response *QueryJoinedHostsInRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryJoinedHostsInRoom")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderQueryJoinedHostsInRoomPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryJoinedHostServerNamesInRoom implements FederationSenderQueryAPI
func (h *httpFederationSenderQueryAPI) QueryJoinedHostServerNamesInRoom(
	ctx context.Context,
	request *QueryJoinedHostServerNamesInRoomRequest,
	response *QueryJoinedHostServerNamesInRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryJoinedHostServerNamesInRoom")
	defer span.Finish()

	apiURL := h.federationSenderURL + FederationSenderQueryJoinedHostServerNamesInRoomPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
