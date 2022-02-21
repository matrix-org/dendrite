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
	// needed to avoid chicken and egg scenario when setting up the
	// interdependencies between the roomserver and other input APIs
	SetFederationAPI(fsAPI fsAPI.FederationInternalAPI, keyRing *gomatrixserverlib.KeyRing)
	SetAppserviceAPI(asAPI asAPI.AppServiceQueryAPI)
	SetUserAPI(userAPI userapi.UserInternalAPI)

	InputRoomEvents(
		ctx context.Context,
		request *InputRoomEventsRequest,
		response *InputRoomEventsResponse,
	)

	PerformInvite(
		ctx context.Context,
		req *PerformInviteRequest,
		res *PerformInviteResponse,
	) error

	PerformJoin(
		ctx context.Context,
		req *PerformJoinRequest,
		res *PerformJoinResponse,
	)

	PerformLeave(
		ctx context.Context,
		req *PerformLeaveRequest,
		res *PerformLeaveResponse,
	) error

	PerformPeek(
		ctx context.Context,
		req *PerformPeekRequest,
		res *PerformPeekResponse,
	)

	PerformUnpeek(
		ctx context.Context,
		req *PerformUnpeekRequest,
		res *PerformUnpeekResponse,
	)

	PerformPublish(
		ctx context.Context,
		req *PerformPublishRequest,
		res *PerformPublishResponse,
	)

	PerformInboundPeek(
		ctx context.Context,
		req *PerformInboundPeekRequest,
		res *PerformInboundPeekResponse,
	) error

	QueryPublishedRooms(
		ctx context.Context,
		req *QueryPublishedRoomsRequest,
		res *QueryPublishedRoomsResponse,
	) error

	// Query the latest events and state for a room from the room server.
	QueryLatestEventsAndState(
		ctx context.Context,
		request *QueryLatestEventsAndStateRequest,
		response *QueryLatestEventsAndStateResponse,
	) error

	// Query the state after a list of events in a room from the room server.
	QueryStateAfterEvents(
		ctx context.Context,
		request *QueryStateAfterEventsRequest,
		response *QueryStateAfterEventsResponse,
	) error

	// Query a list of events by event ID.
	QueryEventsByID(
		ctx context.Context,
		request *QueryEventsByIDRequest,
		response *QueryEventsByIDResponse,
	) error

	// Query the membership event for an user for a room.
	QueryMembershipForUser(
		ctx context.Context,
		request *QueryMembershipForUserRequest,
		response *QueryMembershipForUserResponse,
	) error

	// Query a list of membership events for a room
	QueryMembershipsForRoom(
		ctx context.Context,
		request *QueryMembershipsForRoomRequest,
		response *QueryMembershipsForRoomResponse,
	) error

	// Query if we think we're still in a room.
	QueryServerJoinedToRoom(
		ctx context.Context,
		request *QueryServerJoinedToRoomRequest,
		response *QueryServerJoinedToRoomResponse,
	) error

	// Query whether a server is allowed to see an event
	QueryServerAllowedToSeeEvent(
		ctx context.Context,
		request *QueryServerAllowedToSeeEventRequest,
		response *QueryServerAllowedToSeeEventResponse,
	) error

	// Query missing events for a room from roomserver
	QueryMissingEvents(
		ctx context.Context,
		request *QueryMissingEventsRequest,
		response *QueryMissingEventsResponse,
	) error

	// Query to get state and auth chain for a (potentially hypothetical) event.
	// Takes lists of PrevEventIDs and AuthEventsIDs and uses them to calculate
	// the state and auth chain to return.
	QueryStateAndAuthChain(
		ctx context.Context,
		request *QueryStateAndAuthChainRequest,
		response *QueryStateAndAuthChainResponse,
	) error

	// QueryAuthChain returns the entire auth chain for the event IDs given.
	// The response includes the events in the request.
	// Omits without error for any missing auth events. There will be no duplicates.
	QueryAuthChain(
		ctx context.Context,
		request *QueryAuthChainRequest,
		response *QueryAuthChainResponse,
	) error

	// QueryCurrentState retrieves the requested state events. If state events are not found, they will be missing from
	// the response.
	QueryCurrentState(ctx context.Context, req *QueryCurrentStateRequest, res *QueryCurrentStateResponse) error
	// QueryRoomsForUser retrieves a list of room IDs matching the given query.
	QueryRoomsForUser(ctx context.Context, req *QueryRoomsForUserRequest, res *QueryRoomsForUserResponse) error
	// QueryBulkStateContent does a bulk query for state event content in the given rooms.
	QueryBulkStateContent(ctx context.Context, req *QueryBulkStateContentRequest, res *QueryBulkStateContentResponse) error
	// QuerySharedUsers returns a list of users who share at least 1 room in common with the given user.
	QuerySharedUsers(ctx context.Context, req *QuerySharedUsersRequest, res *QuerySharedUsersResponse) error
	// QueryKnownUsers returns a list of users that we know about from our joined rooms.
	QueryKnownUsers(ctx context.Context, req *QueryKnownUsersRequest, res *QueryKnownUsersResponse) error
	// QueryServerBannedFromRoom returns whether a server is banned from a room by server ACLs.
	QueryServerBannedFromRoom(ctx context.Context, req *QueryServerBannedFromRoomRequest, res *QueryServerBannedFromRoomResponse) error

	// Query a given amount (or less) of events prior to a given set of events.
	PerformBackfill(
		ctx context.Context,
		request *PerformBackfillRequest,
		response *PerformBackfillResponse,
	) error

	// PerformForget forgets a rooms history for a specific user
	PerformForget(ctx context.Context, req *PerformForgetRequest, resp *PerformForgetResponse) error

	// Asks for the default room version as preferred by the server.
	QueryRoomVersionCapabilities(
		ctx context.Context,
		request *QueryRoomVersionCapabilitiesRequest,
		response *QueryRoomVersionCapabilitiesResponse,
	) error

	// Asks for the room version for a given room.
	QueryRoomVersionForRoom(
		ctx context.Context,
		request *QueryRoomVersionForRoomRequest,
		response *QueryRoomVersionForRoomResponse,
	) error

	// Set a room alias
	SetRoomAlias(
		ctx context.Context,
		req *SetRoomAliasRequest,
		response *SetRoomAliasResponse,
	) error

	// Get the room ID for an alias
	GetRoomIDForAlias(
		ctx context.Context,
		req *GetRoomIDForAliasRequest,
		response *GetRoomIDForAliasResponse,
	) error

	// Get all known aliases for a room ID
	GetAliasesForRoomID(
		ctx context.Context,
		req *GetAliasesForRoomIDRequest,
		response *GetAliasesForRoomIDResponse,
	) error

	// Get the user ID of the creator of an alias
	GetCreatorIDForAlias(
		ctx context.Context,
		req *GetCreatorIDForAliasRequest,
		response *GetCreatorIDForAliasResponse,
	) error

	// Remove a room alias
	RemoveRoomAlias(
		ctx context.Context,
		req *RemoveRoomAliasRequest,
		response *RemoveRoomAliasResponse,
	) error
}
