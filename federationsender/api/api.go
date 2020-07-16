package api

import (
	"context"

	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
)

// FederationSenderInternalAPI is used to query information from the federation sender.
type FederationSenderInternalAPI interface {
	// PerformDirectoryLookup looks up a remote room ID from a room alias.
	PerformDirectoryLookup(
		ctx context.Context,
		request *PerformDirectoryLookupRequest,
		response *PerformDirectoryLookupResponse,
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
	)
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
	// Broadcasts an EDU to all servers in rooms we are joined to.
	PerformBroadcastEDU(
		ctx context.Context,
		request *PerformBroadcastEDURequest,
		response *PerformBroadcastEDUResponse,
	) error
}

type PerformDirectoryLookupRequest struct {
	RoomAlias  string                       `json:"room_alias"`
	ServerName gomatrixserverlib.ServerName `json:"server_name"`
}

type PerformDirectoryLookupResponse struct {
	RoomID      string                         `json:"room_id"`
	ServerNames []gomatrixserverlib.ServerName `json:"server_names"`
}

type PerformJoinRequest struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
	// The sorted list of servers to try. Servers will be tried sequentially, after de-duplication.
	ServerNames types.ServerNames      `json:"server_names"`
	Content     map[string]interface{} `json:"content"`
}

type PerformJoinResponse struct {
	LastError *gomatrix.HTTPError
}

type PerformLeaveRequest struct {
	RoomID      string            `json:"room_id"`
	UserID      string            `json:"user_id"`
	ServerNames types.ServerNames `json:"server_names"`
}

type PerformLeaveResponse struct {
}

type PerformServersAliveRequest struct {
	Servers []gomatrixserverlib.ServerName
}

type PerformServersAliveResponse struct {
}

// QueryJoinedHostServerNamesInRoomRequest is a request to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomRequest struct {
	RoomID string `json:"room_id"`
}

// QueryJoinedHostServerNamesInRoomResponse is a response to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomResponse struct {
	ServerNames []gomatrixserverlib.ServerName `json:"server_names"`
}

type PerformBroadcastEDURequest struct {
}

type PerformBroadcastEDUResponse struct {
}
