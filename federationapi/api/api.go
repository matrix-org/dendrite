package api

import (
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/matrix-org/dendrite/federationapi/types"
	rstypes "github.com/matrix-org/dendrite/roomserver/types"
)

// FederationInternalAPI is used to query information from the federation sender.
type FederationInternalAPI interface {
	gomatrixserverlib.FederatedStateClient
	gomatrixserverlib.FederatedJoinClient
	KeyserverFederationAPI
	gomatrixserverlib.KeyDatabase
	ClientFederationAPI
	RoomserverFederationAPI
	P2PFederationAPI

	QueryServerKeys(ctx context.Context, request *QueryServerKeysRequest, response *QueryServerKeysResponse) error
	LookupServerKeys(ctx context.Context, s spec.ServerName, keyRequests map[gomatrixserverlib.PublicKeyLookupRequest]spec.Timestamp) ([]gomatrixserverlib.ServerKeys, error)
	MSC2836EventRelationships(ctx context.Context, origin, dst spec.ServerName, r fclient.MSC2836EventRelationshipsRequest, roomVersion gomatrixserverlib.RoomVersion) (res fclient.MSC2836EventRelationshipsResponse, err error)

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
	SendInvite(ctx context.Context, event gomatrixserverlib.PDU, strippedState []gomatrixserverlib.InviteStrippedState) (gomatrixserverlib.PDU, error)
	// Handle sending an invite to a remote server.
	SendInviteV3(ctx context.Context, event gomatrixserverlib.ProtoEvent, invitee spec.UserID, version gomatrixserverlib.RoomVersion, strippedState []gomatrixserverlib.InviteStrippedState) (gomatrixserverlib.PDU, error)
	// Handle an instruction to peek a room on a remote server.
	PerformOutboundPeek(ctx context.Context, request *PerformOutboundPeekRequest, response *PerformOutboundPeekResponse) error
	// Query the server names of the joined hosts in a room.
	// Unlike QueryJoinedHostsInRoom, this function returns a de-duplicated slice
	// containing only the server names (without information for membership events).
	// The response will include this server if they are joined to the room.
	QueryJoinedHostServerNamesInRoom(ctx context.Context, request *QueryJoinedHostServerNamesInRoomRequest, response *QueryJoinedHostServerNamesInRoomResponse) error
	GetEventAuth(ctx context.Context, origin, s spec.ServerName, roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string) (res fclient.RespEventAuth, err error)
	GetEvent(ctx context.Context, origin, s spec.ServerName, eventID string) (res gomatrixserverlib.Transaction, err error)
	LookupMissingEvents(ctx context.Context, origin, s spec.ServerName, roomID string, missing fclient.MissingEvents, roomVersion gomatrixserverlib.RoomVersion) (res fclient.RespMissingEvents, err error)

	RoomHierarchies(ctx context.Context, origin, dst spec.ServerName, roomID string, suggestedOnly bool) (res fclient.RoomHierarchyResponse, err error)
}

type P2PFederationAPI interface {
	// Get the relay servers associated for the given server.
	P2PQueryRelayServers(
		ctx context.Context,
		request *P2PQueryRelayServersRequest,
		response *P2PQueryRelayServersResponse,
	) error

	// Add relay server associations to the given server.
	P2PAddRelayServers(
		ctx context.Context,
		request *P2PAddRelayServersRequest,
		response *P2PAddRelayServersResponse,
	) error

	// Remove relay server associations from the given server.
	P2PRemoveRelayServers(
		ctx context.Context,
		request *P2PRemoveRelayServersRequest,
		response *P2PRemoveRelayServersResponse,
	) error
}

// KeyserverFederationAPI is a subset of gomatrixserverlib.FederationClient functions which the keyserver
// implements as proxy calls, with built-in backoff/retries/etc. Errors returned from functions in
// this interface are of type FederationClientError
type KeyserverFederationAPI interface {
	GetUserDevices(ctx context.Context, origin, s spec.ServerName, userID string) (res fclient.RespUserDevices, err error)
	ClaimKeys(ctx context.Context, origin, s spec.ServerName, oneTimeKeys map[string]map[string]string) (res fclient.RespClaimKeys, err error)
	QueryKeys(ctx context.Context, origin, s spec.ServerName, keys map[string][]string) (res fclient.RespQueryKeys, err error)
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
	ServerName      spec.ServerName
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
	Requests map[gomatrixserverlib.PublicKeyLookupRequest]spec.Timestamp `json:"requests"`
}

type QueryPublicKeysResponse struct {
	Results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult `json:"results"`
}

type PerformDirectoryLookupRequest struct {
	RoomAlias  string          `json:"room_alias"`
	ServerName spec.ServerName `json:"server_name"`
}

type PerformDirectoryLookupResponse struct {
	RoomID      string            `json:"room_id"`
	ServerNames []spec.ServerName `json:"server_names"`
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
	JoinedVia spec.ServerName
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
	RoomVersion     gomatrixserverlib.RoomVersion           `json:"room_version"`
	Event           *rstypes.HeaderedEvent                  `json:"event"`
	InviteRoomState []gomatrixserverlib.InviteStrippedState `json:"invite_room_state"`
}

type PerformInviteResponse struct {
	Event *rstypes.HeaderedEvent `json:"event"`
}

// QueryJoinedHostServerNamesInRoomRequest is a request to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomRequest struct {
	RoomID             string `json:"room_id"`
	ExcludeSelf        bool   `json:"exclude_self"`
	ExcludeBlacklisted bool   `json:"exclude_blacklisted"`
}

// QueryJoinedHostServerNamesInRoomResponse is a response to QueryJoinedHostServerNames
type QueryJoinedHostServerNamesInRoomResponse struct {
	ServerNames []spec.ServerName `json:"server_names"`
}

type PerformBroadcastEDURequest struct {
}

type PerformBroadcastEDUResponse struct {
}

type PerformWakeupServersRequest struct {
	ServerNames []spec.ServerName `json:"server_names"`
}

type PerformWakeupServersResponse struct {
}

type InputPublicKeysRequest struct {
	Keys map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult `json:"keys"`
}

type InputPublicKeysResponse struct {
}

type P2PQueryRelayServersRequest struct {
	Server spec.ServerName
}

type P2PQueryRelayServersResponse struct {
	RelayServers []spec.ServerName
}

type P2PAddRelayServersRequest struct {
	Server       spec.ServerName
	RelayServers []spec.ServerName
}

type P2PAddRelayServersResponse struct {
}

type P2PRemoveRelayServersRequest struct {
	Server       spec.ServerName
	RelayServers []spec.ServerName
}

type P2PRemoveRelayServersResponse struct {
}
