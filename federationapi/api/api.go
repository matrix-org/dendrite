package api

import (
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/federationapi/types"
)

// FederationInternalAPI is used to query information from the federation sender.
type FederationInternalAPI interface {
	gomatrixserverlib.FederatedStateClient
	KeyserverFederationAPI
	gomatrixserverlib.KeyDatabase
	ClientFederationAPI
	RoomserverFederationAPI

	QueryServerKeys(ctx context.Context, request *QueryServerKeysRequest, response *QueryServerKeysResponse) error
	LookupServerKeys(ctx context.Context, s gomatrixserverlib.ServerName, keyRequests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp) ([]gomatrixserverlib.ServerKeys, error)
	MSC2836EventRelationships(ctx context.Context, origin, dst gomatrixserverlib.ServerName, r gomatrixserverlib.MSC2836EventRelationshipsRequest, roomVersion gomatrixserverlib.RoomVersion) (res gomatrixserverlib.MSC2836EventRelationshipsResponse, err error)
	MSC2946Spaces(ctx context.Context, origin, dst gomatrixserverlib.ServerName, roomID string, suggestedOnly bool) (res gomatrixserverlib.MSC2946SpacesResponse, err error)

	// Broadcasts an EDU to all servers in rooms we are joined to. Used in the yggdrasil demos.
	PerformBroadcastEDU(
		ctx context.Context,
		request *PerformBroadcastEDURequest,
		response *PerformBroadcastEDUResponse,
	) error

	PerformWakeupServers(
		ctx context.Context,
		request *PerformWakeupServersRequest,
		response *PerformWakeupServersResponse,
	) error
}

type ClientFederationAPI interface {
	// Query the server names of the joined hosts in a room.
	// Unlike QueryJoinedHostsInRoom, this function returns a de-duplicated slice
	// containing only the server names (without information for membership events).
	// The response will include this server if they are joined to the room.
	QueryJoinedHostServerNamesInRoom(ctx context.Context, request *QueryJoinedHostServerNamesInRoomRequest, response *QueryJoinedHostServerNamesInRoomResponse) error
}

type RoomserverFederationAPI interface {
	gomatrixserverlib.BackfillClient
	gomatrixserverlib.FederatedStateClient
	KeyRing() *gomatrixserverlib.KeyRing

	// PerformDirectoryLookup looks up a remote room ID from a room alias.
	PerformDirectoryLookup(ctx context.Context, request *PerformDirectoryLookupRequest, response *PerformDirectoryLookupResponse) error
	// Handle an instruction to make_join & send_join with a remote server.
	PerformJoin(ctx context.Context, request *PerformJoinRequest, response *PerformJoinResponse)
	// Handle an instruction to make_leave & send_leave with a remote server.
	PerformLeave(ctx context.Context, request *PerformLeaveRequest, response *PerformLeaveResponse) error
	// Handle sending an invite to a remote server.
	PerformInvite(ctx context.Context, request *PerformInviteRequest, response *PerformInviteResponse) error
	// Handle an instruction to peek a room on a remote server.
	PerformOutboundPeek(ctx context.Context, request *PerformOutboundPeekRequest, response *PerformOutboundPeekResponse) error
	// Query the server names of the joined hosts in a room.
	// Unlike QueryJoinedHostsInRoom, this function returns a de-duplicated slice
	// containing only the server names (without information for membership events).
	// The response will include this server if they are joined to the room.
	QueryJoinedHostServerNamesInRoom(ctx context.Context, request *QueryJoinedHostServerNamesInRoomRequest, response *QueryJoinedHostServerNamesInRoomResponse) error
	GetEventAuth(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string) (res gomatrixserverlib.RespEventAuth, err error)
	GetEvent(ctx context.Context, origin, s gomatrixserverlib.ServerName, eventID string) (res gomatrixserverlib.Transaction, err error)
	LookupMissingEvents(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID string, missing gomatrixserverlib.MissingEvents, roomVersion gomatrixserverlib.RoomVersion) (res gomatrixserverlib.RespMissingEvents, err error)
}

// KeyserverFederationAPI is a subset of gomatrixserverlib.FederationClient functions which the keyserver
// implements as proxy calls, with built-in backoff/retries/etc. Errors returned from functions in
// this interface are of type FederationClientError
type KeyserverFederationAPI interface {
	GetUserDevices(ctx context.Context, origin, s gomatrixserverlib.ServerName, userID string) (res gomatrixserverlib.RespUserDevices, err error)
	ClaimKeys(ctx context.Context, origin, s gomatrixserverlib.ServerName, oneTimeKeys map[string]map[string]string) (res gomatrixserverlib.RespClaimKeys, err error)
	QueryKeys(ctx context.Context, origin, s gomatrixserverlib.ServerName, keys map[string][]string) (res gomatrixserverlib.RespQueryKeys, err error)
}

// an interface for gmsl.FederationClient - contains functions called by federationapi only.
type FederationClient interface {
	gomatrixserverlib.KeyClient
	SendTransaction(ctx context.Context, t gomatrixserverlib.Transaction) (res gomatrixserverlib.RespSend, err error)

	// Perform operations
	LookupRoomAlias(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomAlias string) (res gomatrixserverlib.RespDirectory, err error)
	Peek(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID, peekID string, roomVersions []gomatrixserverlib.RoomVersion) (res gomatrixserverlib.RespPeek, err error)
	MakeJoin(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID, userID string, roomVersions []gomatrixserverlib.RoomVersion) (res gomatrixserverlib.RespMakeJoin, err error)
	SendJoin(ctx context.Context, origin, s gomatrixserverlib.ServerName, event *gomatrixserverlib.Event) (res gomatrixserverlib.RespSendJoin, err error)
	MakeLeave(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID, userID string) (res gomatrixserverlib.RespMakeLeave, err error)
	SendLeave(ctx context.Context, origin, s gomatrixserverlib.ServerName, event *gomatrixserverlib.Event) (err error)
	SendInviteV2(ctx context.Context, origin, s gomatrixserverlib.ServerName, request gomatrixserverlib.InviteV2Request) (res gomatrixserverlib.RespInviteV2, err error)

	GetEvent(ctx context.Context, origin, s gomatrixserverlib.ServerName, eventID string) (res gomatrixserverlib.Transaction, err error)

	GetEventAuth(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string) (res gomatrixserverlib.RespEventAuth, err error)
	GetUserDevices(ctx context.Context, origin, s gomatrixserverlib.ServerName, userID string) (gomatrixserverlib.RespUserDevices, error)
	ClaimKeys(ctx context.Context, origin, s gomatrixserverlib.ServerName, oneTimeKeys map[string]map[string]string) (gomatrixserverlib.RespClaimKeys, error)
	QueryKeys(ctx context.Context, origin, s gomatrixserverlib.ServerName, keys map[string][]string) (gomatrixserverlib.RespQueryKeys, error)
	Backfill(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID string, limit int, eventIDs []string) (res gomatrixserverlib.Transaction, err error)
	MSC2836EventRelationships(ctx context.Context, origin, dst gomatrixserverlib.ServerName, r gomatrixserverlib.MSC2836EventRelationshipsRequest, roomVersion gomatrixserverlib.RoomVersion) (res gomatrixserverlib.MSC2836EventRelationshipsResponse, err error)
	MSC2946Spaces(ctx context.Context, origin, dst gomatrixserverlib.ServerName, roomID string, suggestedOnly bool) (res gomatrixserverlib.MSC2946SpacesResponse, err error)

	ExchangeThirdPartyInvite(ctx context.Context, origin, s gomatrixserverlib.ServerName, builder gomatrixserverlib.EventBuilder) (err error)
	LookupState(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID string, eventID string, roomVersion gomatrixserverlib.RoomVersion) (res gomatrixserverlib.RespState, err error)
	LookupStateIDs(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID string, eventID string) (res gomatrixserverlib.RespStateIDs, err error)
	LookupMissingEvents(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID string, missing gomatrixserverlib.MissingEvents, roomVersion gomatrixserverlib.RoomVersion) (res gomatrixserverlib.RespMissingEvents, err error)
}

// FederationClientError is returned from FederationClient methods in the event of a problem.
type FederationClientError struct {
	Err         string
	RetryAfter  time.Duration
	Blacklisted bool
	Code        int // HTTP Status code from the remote server
}

func (e FederationClientError) Error() string {
	return fmt.Sprintf("%s - (retry_after=%s, blacklisted=%v)", e.Err, e.RetryAfter.String(), e.Blacklisted)
}

type QueryServerKeysRequest struct {
	ServerName      gomatrixserverlib.ServerName
	KeyIDToCriteria map[gomatrixserverlib.KeyID]gomatrixserverlib.PublicKeyNotaryQueryCriteria
}

func (q *QueryServerKeysRequest) KeyIDs() []gomatrixserverlib.KeyID {
	kids := make([]gomatrixserverlib.KeyID, len(q.KeyIDToCriteria))
	i := 0
	for keyID := range q.KeyIDToCriteria {
		kids[i] = keyID
		i++
	}
	return kids
}

type QueryServerKeysResponse struct {
	ServerKeys []gomatrixserverlib.ServerKeys
}

type QueryPublicKeysRequest struct {
	Requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp `json:"requests"`
}

type QueryPublicKeysResponse struct {
	Results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult `json:"results"`
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
	Unsigned    map[string]interface{} `json:"unsigned"`
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

// QueryJoinedHostServerNamesInRoomRequest is a request to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomRequest struct {
	RoomID             string `json:"room_id"`
	ExcludeSelf        bool   `json:"exclude_self"`
	ExcludeBlacklisted bool   `json:"exclude_blacklisted"`
}

// QueryJoinedHostServerNamesInRoomResponse is a response to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomResponse struct {
	ServerNames []gomatrixserverlib.ServerName `json:"server_names"`
}

type PerformBroadcastEDURequest struct {
}

type PerformBroadcastEDUResponse struct {
}

type PerformWakeupServersRequest struct {
	ServerNames []gomatrixserverlib.ServerName `json:"server_names"`
}

type PerformWakeupServersResponse struct {
}

type InputPublicKeysRequest struct {
	Keys map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult `json:"keys"`
}

type InputPublicKeysResponse struct {
}
