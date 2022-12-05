package api

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"

	asAPI "github.com/matrix-org/dendrite/appservice/api"
	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// RoomserverInputAPI is used to write events to the room server.
type RoomserverInternalAPI interface {
	SyncRoomserverAPI
	AppserviceRoomserverAPI
	ClientRoomserverAPI
	UserRoomserverAPI
	FederationRoomserverAPI

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

type InputRoomEventsAPI interface {
	InputRoomEvents(
		ctx context.Context,
		req *InputRoomEventsRequest,
		res *InputRoomEventsResponse,
	) error
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
	// Query a list of events by event ID.
	QueryEventsByID(
		ctx context.Context,
		req *QueryEventsByIDRequest,
		res *QueryEventsByIDResponse,
	) error
	// QueryCurrentState retrieves the requested state events. If state events are not found, they will be missing from
	// the response.
	QueryCurrentState(ctx context.Context, req *QueryCurrentStateRequest, res *QueryCurrentStateResponse) error
}

// API functions required by the syncapi
type SyncRoomserverAPI interface {
	QueryLatestEventsAndStateAPI
	QueryBulkStateContentAPI
	// QuerySharedUsers returns a list of users who share at least 1 room in common with the given user.
	QuerySharedUsers(ctx context.Context, req *QuerySharedUsersRequest, res *QuerySharedUsersResponse) error
	// Query a list of events by event ID.
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
	// Returns a map from eventID to a slice of gomatrixserverlib.HeaderedEvent.
	QueryMembershipAtEvent(
		ctx context.Context,
		request *QueryMembershipAtEventRequest,
		response *QueryMembershipAtEventResponse,
	) error
}

type AppserviceRoomserverAPI interface {
	// Query a list of events by event ID.
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
	QueryMembershipForUser(ctx context.Context, req *QueryMembershipForUserRequest, res *QueryMembershipForUserResponse) error
	QueryMembershipsForRoom(ctx context.Context, req *QueryMembershipsForRoomRequest, res *QueryMembershipsForRoomResponse) error
	QueryRoomsForUser(ctx context.Context, req *QueryRoomsForUserRequest, res *QueryRoomsForUserResponse) error
	QueryStateAfterEvents(ctx context.Context, req *QueryStateAfterEventsRequest, res *QueryStateAfterEventsResponse) error
	// QueryKnownUsers returns a list of users that we know about from our joined rooms.
	QueryKnownUsers(ctx context.Context, req *QueryKnownUsersRequest, res *QueryKnownUsersResponse) error
	QueryRoomVersionForRoom(ctx context.Context, req *QueryRoomVersionForRoomRequest, res *QueryRoomVersionForRoomResponse) error
	QueryPublishedRooms(ctx context.Context, req *QueryPublishedRoomsRequest, res *QueryPublishedRoomsResponse) error
	QueryRoomVersionCapabilities(ctx context.Context, req *QueryRoomVersionCapabilitiesRequest, res *QueryRoomVersionCapabilitiesResponse) error

	GetRoomIDForAlias(ctx context.Context, req *GetRoomIDForAliasRequest, res *GetRoomIDForAliasResponse) error
	GetAliasesForRoomID(ctx context.Context, req *GetAliasesForRoomIDRequest, res *GetAliasesForRoomIDResponse) error

	// PerformRoomUpgrade upgrades a room to a newer version
	PerformRoomUpgrade(ctx context.Context, req *PerformRoomUpgradeRequest, resp *PerformRoomUpgradeResponse) error
	PerformAdminEvacuateRoom(ctx context.Context, req *PerformAdminEvacuateRoomRequest, res *PerformAdminEvacuateRoomResponse) error
	PerformAdminEvacuateUser(ctx context.Context, req *PerformAdminEvacuateUserRequest, res *PerformAdminEvacuateUserResponse) error
	PerformAdminDownloadState(ctx context.Context, req *PerformAdminDownloadStateRequest, res *PerformAdminDownloadStateResponse) error
	PerformPeek(ctx context.Context, req *PerformPeekRequest, res *PerformPeekResponse) error
	PerformUnpeek(ctx context.Context, req *PerformUnpeekRequest, res *PerformUnpeekResponse) error
	PerformInvite(ctx context.Context, req *PerformInviteRequest, res *PerformInviteResponse) error
	PerformJoin(ctx context.Context, req *PerformJoinRequest, res *PerformJoinResponse) error
	PerformLeave(ctx context.Context, req *PerformLeaveRequest, res *PerformLeaveResponse) error
	PerformPublish(ctx context.Context, req *PerformPublishRequest, res *PerformPublishResponse) error
	// PerformForget forgets a rooms history for a specific user
	PerformForget(ctx context.Context, req *PerformForgetRequest, resp *PerformForgetResponse) error
	SetRoomAlias(ctx context.Context, req *SetRoomAliasRequest, res *SetRoomAliasResponse) error
	RemoveRoomAlias(ctx context.Context, req *RemoveRoomAliasRequest, res *RemoveRoomAliasResponse) error
}

type UserRoomserverAPI interface {
	QueryLatestEventsAndStateAPI
	QueryCurrentState(ctx context.Context, req *QueryCurrentStateRequest, res *QueryCurrentStateResponse) error
	QueryMembershipsForRoom(ctx context.Context, req *QueryMembershipsForRoomRequest, res *QueryMembershipsForRoomResponse) error
	PerformAdminEvacuateUser(ctx context.Context, req *PerformAdminEvacuateUserRequest, res *PerformAdminEvacuateUserResponse) error
	PerformJoin(ctx context.Context, req *PerformJoinRequest, res *PerformJoinResponse) error
}

type FederationRoomserverAPI interface {
	InputRoomEventsAPI
	QueryLatestEventsAndStateAPI
	QueryBulkStateContentAPI
	// QueryServerBannedFromRoom returns whether a server is banned from a room by server ACLs.
	QueryServerBannedFromRoom(ctx context.Context, req *QueryServerBannedFromRoomRequest, res *QueryServerBannedFromRoomResponse) error
	QueryMembershipsForRoom(ctx context.Context, req *QueryMembershipsForRoomRequest, res *QueryMembershipsForRoomResponse) error
	QueryRoomVersionForRoom(ctx context.Context, req *QueryRoomVersionForRoomRequest, res *QueryRoomVersionForRoomResponse) error
	GetRoomIDForAlias(ctx context.Context, req *GetRoomIDForAliasRequest, res *GetRoomIDForAliasResponse) error
	QueryEventsByID(ctx context.Context, req *QueryEventsByIDRequest, res *QueryEventsByIDResponse) error
	// Query to get state and auth chain for a (potentially hypothetical) event.
	// Takes lists of PrevEventIDs and AuthEventsIDs and uses them to calculate
	// the state and auth chain to return.
	QueryStateAndAuthChain(ctx context.Context, req *QueryStateAndAuthChainRequest, res *QueryStateAndAuthChainResponse) error
	// Query if we think we're still in a room.
	QueryServerJoinedToRoom(ctx context.Context, req *QueryServerJoinedToRoomRequest, res *QueryServerJoinedToRoomResponse) error
	QueryPublishedRooms(ctx context.Context, req *QueryPublishedRoomsRequest, res *QueryPublishedRoomsResponse) error
	// Query missing events for a room from roomserver
	QueryMissingEvents(ctx context.Context, req *QueryMissingEventsRequest, res *QueryMissingEventsResponse) error
	// Query whether a server is allowed to see an event
	QueryServerAllowedToSeeEvent(ctx context.Context, req *QueryServerAllowedToSeeEventRequest, res *QueryServerAllowedToSeeEventResponse) error
	QueryRoomsForUser(ctx context.Context, req *QueryRoomsForUserRequest, res *QueryRoomsForUserResponse) error
	QueryRestrictedJoinAllowed(ctx context.Context, req *QueryRestrictedJoinAllowedRequest, res *QueryRestrictedJoinAllowedResponse) error
	PerformInboundPeek(ctx context.Context, req *PerformInboundPeekRequest, res *PerformInboundPeekResponse) error
	PerformInvite(ctx context.Context, req *PerformInviteRequest, res *PerformInviteResponse) error
	// Query a given amount (or less) of events prior to a given set of events.
	PerformBackfill(ctx context.Context, req *PerformBackfillRequest, res *PerformBackfillResponse) error
}
