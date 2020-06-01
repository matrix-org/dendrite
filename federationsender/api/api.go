package api

import (
	"context"
	"errors"
	"net/http"
)

// FederationSenderInternalAPI is used to query information from the federation sender.
type FederationSenderInternalAPI interface {
	// PerformDirectoryLookup looks up a remote room ID from a room alias.
	PerformDirectoryLookup(
		ctx context.Context,
		request *PerformDirectoryLookupRequest,
		response *PerformDirectoryLookupResponse,
	) error
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
	// Handle an instruction to make_join & send_join with a remote server.
	PerformJoin(
		ctx context.Context,
		request *PerformJoinRequest,
		response *PerformJoinResponse,
	) error
	// Handle an instruction to make_leave & send_leave with a remote server.
	PerformLeave(
		ctx context.Context,
		request *PerformLeaveRequest,
		response *PerformLeaveResponse,
	) error
	// Notifies the federation sender that these servers may be online and to retry sending messages.
	PerformServersAlive(
		ctx context.Context,
		request *PerformServersAliveRequest,
		response *PerformServersAliveResponse,
	) error
}

// NewFederationSenderInternalAPIHTTP creates a FederationSenderInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewFederationSenderInternalAPIHTTP(federationSenderURL string, httpClient *http.Client) (FederationSenderInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewFederationSenderInternalAPIHTTP: httpClient is <nil>")
	}
	return &httpFederationSenderInternalAPI{federationSenderURL, httpClient}, nil
}

type httpFederationSenderInternalAPI struct {
	federationSenderURL string
	httpClient          *http.Client
}
