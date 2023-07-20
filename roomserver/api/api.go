package api

import (
	"context"
	"crypto/ed25519"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"

	asAPI "github.com/matrix-org/dendrite/appservice/api"
	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// ErrInvalidID is an error returned if the userID is invalid
type ErrInvalidID struct {
	Err error
}

func (e ErrInvalidID) Error() string {
	return e.Err.Error()
}

// ErrNotAllowed is an error returned if the user is not allowed
// to execute some action (e.g. invite)
type ErrNotAllowed struct {
	Err error
}

func (e ErrNotAllowed) Error() string {
	return e.Err.Error()
}

// ErrRoomUnknownOrNotAllowed is an error return if either the provided
// room ID does not exist, or points to a room that the requester does
// not have access to.
type ErrRoomUnknownOrNotAllowed struct {
	Err error
}

func (e ErrRoomUnknownOrNotAllowed) Error() string {
	return e.Err.Error()
}

type RestrictedJoinAPI interface {
	CurrentStateEvent(ctx context.Context, roomID spec.RoomID, eventType string, stateKey string) (gomatrixserverlib.PDU, error)
	InvitePending(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (bool, error)
	RestrictedRoomJoinInfo(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID, localServerName spec.ServerName) (*gomatrixserverlib.RestrictedRoomJoinInfo, error)
	QueryRoomInfo(ctx context.Context, roomID spec.RoomID) (*types.RoomInfo, error)
	QueryServerJoinedToRoom(ctx context.Context, req *QueryServerJoinedToRoomRequest, res *QueryServerJoinedToRoomResponse) error
	UserJoinedToRoom(ctx context.Context, roomID types.RoomNID, senderID spec.SenderID) (bool, error)
	LocallyJoinedUsers(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, roomNID types.RoomNID) ([]gomatrixserverlib.PDU, error)
}

// RoomserverInputAPI is used to write events to the room server.
type RoomserverInternalAPI interface {
	SyncRoomserverAPI
	AppserviceRoomserverAPI
	ClientRoomserverAPI
	UserRoomserverAPI
	FederationRoomserverAPI
	QuerySenderIDAPI
	UserRoomPrivateKeyCreator

	// needed to avoid chicken and egg scenario when setting up the
	// interdependencies between the roomserver and other input APIs
	SetFederationAPI(fsAPI fsAPI.RoomserverFederationAPI, keyRing *gomatrixserverlib.KeyRing)
	SetAppserviceAPI(asAPI asAPI.AppServiceInternalAPI)
	SetUserAPI(userAPI userapi.RoomserverUserAPI)

	// QueryAuthChain returns the entire auth chain for the event IDs given.
	// The response includes the events in the request.
	// Omits without error for any missing auth events. There will be no duplicates.
	// Used in MSC2836.
	QueryAuthChain(
		ctx context.Context,
		req *QueryAuthChainRequest,
		res *QueryAuthChainResponse,
	) error
}

type UserRoomPrivateKeyCreator interface {
	// GetOrCreateUserRoomPrivateKey gets the user room key for the specified user. If no key exists yet, a new one is created.
	GetOrCreateUserRoomPrivateKey(ctx context.Context, userID spec.UserID, roomID spec.RoomID) (ed25519.PrivateKey, error)
	StoreUserRoomPublicKey(ctx context.Context, senderID spec.SenderID, userID spec.UserID, roomID spec.RoomID) error
}

type InputRoomEventsAPI interface {
	InputRoomEvents(
		ctx context.Context,
		req *InputRoomEventsRequest,
		res *InputRoomEventsResponse,
	)
}

type QuerySenderIDAPI interface {
	QuerySenderIDForUser(ctx context.Context, roomID spec.RoomID, userID spec.UserID) (spec.SenderID, error)
	QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error)
}

// Query the latest events and state for a room from the room server.
type QueryLatestEventsAndStateAPI interface {
	QueryLatestEventsAndState(ctx context.Context, req *QueryLatestEventsAndStateRequest, res *QueryLatestEventsAndStateResponse) error
}

// QueryBulkStateContent does a bulk query for state event content in the given rooms.
type QueryBulkStateContentAPI interface {
	QueryBulkStateContent(ctx context.Context, req *QueryBulkStateContentRequest, res *QueryBulkStateContentResponse) error
}

type QueryEventsAPI interface {
	// QueryEventsByID queries a list of events by event ID for one room. If no room is specified, it will try to determine
	// which room to use by querying the first events roomID.
	QueryEventsByID(
		ctx context.Context,
		req *QueryEventsByIDRequest,
		res *QueryEventsByIDResponse,
	) error
	// QueryCurrentState retrieves the requested state events. If state events are not found, they will be missing from
	// the response.
	QueryCurrentState(ctx context.Context, req *QueryCurrentStateRequest, res *QueryCurrentStateResponse) error
}

type QueryRoomHierarchyAPI interface {
	// Traverse the room hierarchy using the provided walker up to the provided limit,
	// returning a new walker which can be used to fetch the next page.
	//
	// If limit is -1, this is treated as no limit, and the entire hierarchy will be traversed.
	//
	// If returned walker is nil, then there are no more rooms left to traverse. This method does not modify the provided walker, so it
	// can be cached.
	QueryNextRoomHierarchyPage(ctx context.Context, walker RoomHierarchyWalker, limit int) ([]fclient.RoomHierarchyRoom, *RoomHierarchyWalker, error)
}

// API functions required by the syncapi
type SyncRoomserverAPI interface {
	QueryLatestEventsAndStateAPI
	QueryBulkStateContentAPI
	QuerySenderIDAPI
	// QuerySharedUsers returns a list of users who share at least 1 room in common with the given user.
	QuerySharedUsers(ctx context.Context, req *QuerySharedUsersRequest, res *QuerySharedUsersResponse) error
	// QueryEventsByID queries a list of events by event ID for one room. If no room is specified, it will try to determine
	// which room to use by querying the first events roomID.
	QueryEventsByID(
		ctx context.Context,
		req *QueryEventsByIDRequest,
		res *QueryEventsByIDResponse,
	) error
	// Query the membership event for an user for a room.
	QueryMembershipForUser(
		ctx context.Context,
		req *QueryMembershipForUserRequest,
		res *QueryMembershipForUserResponse,
	) error

	// Query the state after a list of events in a room from the room server.
	QueryStateAfterEvents(
		ctx context.Context,
		req *QueryStateAfterEventsRequest,
		res *QueryStateAfterEventsResponse,
	) error

	// Query a given amount (or less) of events prior to a given set of events.
	PerformBackfill(
		ctx context.Context,
		req *PerformBackfillRequest,
		res *PerformBackfillResponse,
	) error

	// QueryMembershipAtEvent queries the memberships at the given events.
	// Returns a map from eventID to a slice of types.HeaderedEvent.
	QueryMembershipAtEvent(
		ctx context.Context,
		request *QueryMembershipAtEventRequest,
		response *QueryMembershipAtEventResponse,
	) error
}

type AppserviceRoomserverAPI interface {
	QuerySenderIDAPI
	// QueryEventsByID queries a list of events by event ID for one room. If no room is specified, it will try to determine
	// which room to use by querying the first events roomID.
	QueryEventsByID(
		ctx context.Context,
		req *QueryEventsByIDRequest,
		res *QueryEventsByIDResponse,
	) error
	// Query a list of membership events for a room
	QueryMembershipsForRoom(
		ctx context.Context,
		req *QueryMembershipsForRoomRequest,
		res *QueryMembershipsForRoomResponse,
	) error
	// Get all known aliases for a room ID
	GetAliasesForRoomID(
		ctx context.Context,
		req *GetAliasesForRoomIDRequest,
		res *GetAliasesForRoomIDResponse,
	) error
}

type ClientRoomserverAPI interface {
	InputRoomEventsAPI
	QueryLatestEventsAndStateAPI
	QueryBulkStateContentAPI
	QueryEventsAPI
	QuerySenderIDAPI
	UserRoomPrivateKeyCreator
	QueryRoomHierarchyAPI
	QueryMembershipForUser(ctx context.Context, req *QueryMembershipForUserRequest, res *QueryMembershipForUserResponse) error
	QueryMembershipsForRoom(ctx context.Context, req *QueryMembershipsForRoomRequest, res *QueryMembershipsForRoomResponse) error
	QueryRoomsForUser(ctx context.Context, req *QueryRoomsForUserRequest, res *QueryRoomsForUserResponse) error
	QueryStateAfterEvents(ctx context.Context, req *QueryStateAfterEventsRequest, res *QueryStateAfterEventsResponse) error
	// QueryKnownUsers returns a list of users that we know about from our joined rooms.
	QueryKnownUsers(ctx context.Context, req *QueryKnownUsersRequest, res *QueryKnownUsersResponse) error
	QueryRoomVersionForRoom(ctx context.Context, roomID string) (gomatrixserverlib.RoomVersion, error)
	QueryPublishedRooms(ctx context.Context, req *QueryPublishedRoomsRequest, res *QueryPublishedRoomsResponse) error

	GetRoomIDForAlias(ctx context.Context, req *GetRoomIDForAliasRequest, res *GetRoomIDForAliasResponse) error
	GetAliasesForRoomID(ctx context.Context, req *GetAliasesForRoomIDRequest, res *GetAliasesForRoomIDResponse) error

	PerformCreateRoom(ctx context.Context, userID spec.UserID, roomID spec.RoomID, createRequest *PerformCreateRoomRequest) (string, *util.JSONResponse)
	// PerformRoomUpgrade upgrades a room to a newer version
	PerformRoomUpgrade(ctx context.Context, roomID string, userID spec.UserID, roomVersion gomatrixserverlib.RoomVersion) (newRoomID string, err error)
	PerformAdminEvacuateRoom(ctx context.Context, roomID string) (affected []string, err error)
	PerformAdminEvacuateUser(ctx context.Context, userID string) (affected []string, err error)
	PerformAdminPurgeRoom(ctx context.Context, roomID string) error
	PerformAdminDownloadState(ctx context.Context, roomID, userID string, serverName spec.ServerName) error
	PerformPeek(ctx context.Context, req *PerformPeekRequest) (roomID string, err error)
	PerformUnpeek(ctx context.Context, roomID, userID, deviceID string) error
	PerformInvite(ctx context.Context, req *PerformInviteRequest) error
	PerformJoin(ctx context.Context, req *PerformJoinRequest) (roomID string, joinedVia spec.ServerName, err error)
	PerformLeave(ctx context.Context, req *PerformLeaveRequest, res *PerformLeaveResponse) error
	PerformPublish(ctx context.Context, req *PerformPublishRequest) error
	// PerformForget forgets a rooms history for a specific user
	PerformForget(ctx context.Context, req *PerformForgetRequest, resp *PerformForgetResponse) error
	SetRoomAlias(ctx context.Context, req *SetRoomAliasRequest, res *SetRoomAliasResponse) error
	RemoveRoomAlias(ctx context.Context, req *RemoveRoomAliasRequest, res *RemoveRoomAliasResponse) error
	SigningIdentityFor(ctx context.Context, roomID spec.RoomID, senderID spec.UserID) (fclient.SigningIdentity, error)
}

type UserRoomserverAPI interface {
	QuerySenderIDAPI
	QueryLatestEventsAndStateAPI
	KeyserverRoomserverAPI
	QueryCurrentState(ctx context.Context, req *QueryCurrentStateRequest, res *QueryCurrentStateResponse) error
	QueryMembershipsForRoom(ctx context.Context, req *QueryMembershipsForRoomRequest, res *QueryMembershipsForRoomResponse) error
	PerformAdminEvacuateUser(ctx context.Context, userID string) (affected []string, err error)
	PerformJoin(ctx context.Context, req *PerformJoinRequest) (roomID string, joinedVia spec.ServerName, err error)
	JoinedUserCount(ctx context.Context, roomID string) (int, error)
}

type FederationRoomserverAPI interface {
	RestrictedJoinAPI
	InputRoomEventsAPI
	QueryLatestEventsAndStateAPI
	QueryBulkStateContentAPI
	QuerySenderIDAPI
	QueryRoomHierarchyAPI
	UserRoomPrivateKeyCreator
	AssignRoomNID(ctx context.Context, roomID spec.RoomID, roomVersion gomatrixserverlib.RoomVersion) (roomNID types.RoomNID, err error)
	SigningIdentityFor(ctx context.Context, roomID spec.RoomID, senderID spec.UserID) (fclient.SigningIdentity, error)
	// QueryServerBannedFromRoom returns whether a server is banned from a room by server ACLs.
	QueryServerBannedFromRoom(ctx context.Context, req *QueryServerBannedFromRoomRequest, res *QueryServerBannedFromRoomResponse) error
	QueryMembershipForUser(ctx context.Context, req *QueryMembershipForUserRequest, res *QueryMembershipForUserResponse) error
	QueryMembershipForSenderID(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID, res *QueryMembershipForUserResponse) error
	QueryMembershipsForRoom(ctx context.Context, req *QueryMembershipsForRoomRequest, res *QueryMembershipsForRoomResponse) error
	QueryRoomVersionForRoom(ctx context.Context, roomID string) (gomatrixserverlib.RoomVersion, error)
	GetRoomIDForAlias(ctx context.Context, req *GetRoomIDForAliasRequest, res *GetRoomIDForAliasResponse) error
	// QueryEventsByID queries a list of events by event ID for one room. If no room is specified, it will try to determine
	// which room to use by querying the first events roomID.
	QueryEventsByID(ctx context.Context, req *QueryEventsByIDRequest, res *QueryEventsByIDResponse) error
	// Query to get state and auth chain for a (potentially hypothetical) event.
	// Takes lists of PrevEventIDs and AuthEventsIDs and uses them to calculate
	// the state and auth chain to return.
	QueryStateAndAuthChain(ctx context.Context, req *QueryStateAndAuthChainRequest, res *QueryStateAndAuthChainResponse) error
	QueryPublishedRooms(ctx context.Context, req *QueryPublishedRoomsRequest, res *QueryPublishedRoomsResponse) error
	// Query missing events for a room from roomserver
	QueryMissingEvents(ctx context.Context, req *QueryMissingEventsRequest, res *QueryMissingEventsResponse) error
	// Query whether a server is allowed to see an event
	QueryServerAllowedToSeeEvent(ctx context.Context, serverName spec.ServerName, eventID string, roomID string) (allowed bool, err error)
	QueryRoomsForUser(ctx context.Context, req *QueryRoomsForUserRequest, res *QueryRoomsForUserResponse) error
	QueryRestrictedJoinAllowed(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (string, error)
	PerformInboundPeek(ctx context.Context, req *PerformInboundPeekRequest, res *PerformInboundPeekResponse) error
	HandleInvite(ctx context.Context, event *types.HeaderedEvent) error

	PerformInvite(ctx context.Context, req *PerformInviteRequest) error
	// Query a given amount (or less) of events prior to a given set of events.
	PerformBackfill(ctx context.Context, req *PerformBackfillRequest, res *PerformBackfillResponse) error

	IsKnownRoom(ctx context.Context, roomID spec.RoomID) (bool, error)
	StateQuerier() gomatrixserverlib.StateQuerier
}

type KeyserverRoomserverAPI interface {
	QueryLeftUsers(ctx context.Context, req *QueryLeftUsersRequest, res *QueryLeftUsersResponse) error
}
