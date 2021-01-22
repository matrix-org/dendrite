package api

import (
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
)

// FederationClient is a subset of gomatrixserverlib.FederationClient functions which the fedsender
// implements as proxy calls, with built-in backoff/retries/etc. Errors returned from functions in
// this interface are of type FederationClientError
type FederationClient interface {
	gomatrixserverlib.BackfillClient
	gomatrixserverlib.FederatedStateClient
	GetUserDevices(ctx context.Context, s gomatrixserverlib.ServerName, userID string) (res gomatrixserverlib.RespUserDevices, err error)
	ClaimKeys(ctx context.Context, s gomatrixserverlib.ServerName, oneTimeKeys map[string]map[string]string) (res gomatrixserverlib.RespClaimKeys, err error)
	QueryKeys(ctx context.Context, s gomatrixserverlib.ServerName, keys map[string][]string) (res gomatrixserverlib.RespQueryKeys, err error)
	GetEvent(ctx context.Context, s gomatrixserverlib.ServerName, eventID string) (res gomatrixserverlib.Transaction, err error)
	GetServerKeys(ctx context.Context, matrixServer gomatrixserverlib.ServerName) (gomatrixserverlib.ServerKeys, error)
	MSC2836EventRelationships(ctx context.Context, dst gomatrixserverlib.ServerName, r gomatrixserverlib.MSC2836EventRelationshipsRequest, roomVersion gomatrixserverlib.RoomVersion) (res gomatrixserverlib.MSC2836EventRelationshipsResponse, err error)
	MSC2946Spaces(ctx context.Context, dst gomatrixserverlib.ServerName, roomID string, r gomatrixserverlib.MSC2946SpacesRequest) (res gomatrixserverlib.MSC2946SpacesResponse, err error)
	LookupServerKeys(ctx context.Context, s gomatrixserverlib.ServerName, keyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp) ([]gomatrixserverlib.ServerKeys, error)
}

// FederationClientError is returned from FederationClient methods in the event of a problem.
type FederationClientError struct {
	Err         string
	RetryAfter  time.Duration
	Blacklisted bool
}

func (e *FederationClientError) Error() string {
	return fmt.Sprintf("%s - (retry_after=%s, blacklisted=%v)", e.Err, e.RetryAfter.String(), e.Blacklisted)
}

// FederationSenderInternalAPI is used to query information from the federation sender.
type FederationSenderInternalAPI interface {
	FederationClient

	// PerformDirectoryLookup looks up a remote room ID from a room alias.
	PerformDirectoryLookup(
		ctx context.Context,
		request *PerformDirectoryLookupRequest,
		response *PerformDirectoryLookupResponse,
	) error
	// Query the server names of the joined hosts in a room.
	// Unlike QueryJoinedHostsInRoom, this function returns a de-duplicated slice
	// containing only the server names (without information for membership events).
	// The response will include this server if they are joined to the room.
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
	// Handle an instruction to peek a room on a remote server.
	PerformOutboundPeek(
		ctx context.Context,
		request *PerformOutboundPeekRequest,
		response *PerformOutboundPeekResponse,
	) error
	// Handle an instruction to make_leave & send_leave with a remote server.
	PerformLeave(
		ctx context.Context,
		request *PerformLeaveRequest,
		response *PerformLeaveResponse,
	) error
	// Handle sending an invite to a remote server.
	PerformInvite(
		ctx context.Context,
		request *PerformInviteRequest,
		response *PerformInviteResponse,
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
	JoinedVia gomatrixserverlib.ServerName
	LastError *gomatrix.HTTPError
}

type PerformOutboundPeekRequest struct {
	RoomID string `json:"room_id"`
	// The sorted list of servers to try. Servers will be tried sequentially, after de-duplication.
	ServerNames types.ServerNames `json:"server_names"`
}

type PerformOutboundPeekResponse struct {
	LastError *gomatrix.HTTPError
}

type PerformLeaveRequest struct {
	RoomID      string            `json:"room_id"`
	UserID      string            `json:"user_id"`
	ServerNames types.ServerNames `json:"server_names"`
}

type PerformLeaveResponse struct {
}

type PerformInviteRequest struct {
	RoomVersion     gomatrixserverlib.RoomVersion             `json:"room_version"`
	Event           *gomatrixserverlib.HeaderedEvent          `json:"event"`
	InviteRoomState []gomatrixserverlib.InviteV2StrippedState `json:"invite_room_state"`
}

type PerformInviteResponse struct {
	Event *gomatrixserverlib.HeaderedEvent `json:"event"`
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
