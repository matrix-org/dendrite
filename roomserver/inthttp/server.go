package inthttp

import (
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
)

// AddRoutes adds the RoomserverInternalAPI handlers to the http.ServeMux.
// nolint: gocyclo
func AddRoutes(r api.RoomserverInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(
		RoomserverInputRoomEventsPath,
		httputil.MakeInternalRPCAPI("InputRoomEvents", r.InputRoomEvents),
	)

	internalAPIMux.Handle(
		RoomserverPerformInvitePath,
		httputil.MakeInternalRPCAPI("PerformInvite", r.PerformInvite),
	)

	internalAPIMux.Handle(
		RoomserverPerformJoinPath,
		httputil.MakeInternalRPCAPI("PerformJoin", r.PerformJoin),
	)

	internalAPIMux.Handle(
		RoomserverPerformLeavePath,
		httputil.MakeInternalRPCAPI("PerformLeave", r.PerformLeave),
	)

	internalAPIMux.Handle(
		RoomserverPerformPeekPath,
		httputil.MakeInternalRPCAPI("PerformPeek", r.PerformPeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformInboundPeekPath,
		httputil.MakeInternalRPCAPI("PerformInboundPeek", r.PerformInboundPeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformUnpeekPath,
		httputil.MakeInternalRPCAPI("PerformUnpeek", r.PerformUnpeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformRoomUpgradePath,
		httputil.MakeInternalRPCAPI("PerformRoomUpgrade", r.PerformRoomUpgrade),
	)

	internalAPIMux.Handle(
		RoomserverPerformPublishPath,
		httputil.MakeInternalRPCAPI("PerformPublish", r.PerformPublish),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminEvacuateRoomPath,
		httputil.MakeInternalRPCAPI("PerformAdminEvacuateRoom", r.PerformAdminEvacuateRoom),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminEvacuateUserPath,
		httputil.MakeInternalRPCAPI("PerformAdminEvacuateUser", r.PerformAdminEvacuateUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryPublishedRoomsPath,
		httputil.MakeInternalRPCAPI("QueryPublishedRooms", r.QueryPublishedRooms),
	)

	internalAPIMux.Handle(
		RoomserverQueryLatestEventsAndStatePath,
		httputil.MakeInternalRPCAPI("QueryLatestEventsAndState", r.QueryLatestEventsAndState),
	)

	internalAPIMux.Handle(
		RoomserverQueryStateAfterEventsPath,
		httputil.MakeInternalRPCAPI("QueryStateAfterEvents", r.QueryStateAfterEvents),
	)

	internalAPIMux.Handle(
		RoomserverQueryEventsByIDPath,
		httputil.MakeInternalRPCAPI("QueryEventsByID", r.QueryEventsByID),
	)

	internalAPIMux.Handle(
		RoomserverQueryMembershipForUserPath,
		httputil.MakeInternalRPCAPI("QueryMembershipForUser", r.QueryMembershipForUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryMembershipsForRoomPath,
		httputil.MakeInternalRPCAPI("QueryMembershipsForRoom", r.QueryMembershipsForRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerJoinedToRoomPath,
		httputil.MakeInternalRPCAPI("QueryServerJoinedToRoom", r.QueryServerJoinedToRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerJoinedToRoomPath,
		httputil.MakeInternalRPCAPI("QueryServerJoinedToRoom", r.QueryServerJoinedToRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerAllowedToSeeEventPath,
		httputil.MakeInternalRPCAPI("QueryServerAllowedToSeeEvent", r.QueryServerAllowedToSeeEvent),
	)

	internalAPIMux.Handle(
		RoomserverQueryMissingEventsPath,
		httputil.MakeInternalRPCAPI("QueryMissingEvents", r.QueryMissingEvents),
	)

	internalAPIMux.Handle(
		RoomserverQueryStateAndAuthChainPath,
		httputil.MakeInternalRPCAPI("QueryStateAndAuthChain", r.QueryStateAndAuthChain),
	)

	internalAPIMux.Handle(
		RoomserverPerformBackfillPath,
		httputil.MakeInternalRPCAPI("PerformBackfill", r.PerformBackfill),
	)

	internalAPIMux.Handle(
		RoomserverPerformForgetPath,
		httputil.MakeInternalRPCAPI("PerformForget", r.PerformForget),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomVersionCapabilitiesPath,
		httputil.MakeInternalRPCAPI("QueryRoomVersionCapabilities", r.QueryRoomVersionCapabilities),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomVersionForRoomPath,
		httputil.MakeInternalRPCAPI("QueryRoomVersionForRoom", r.QueryRoomVersionForRoom),
	)

	internalAPIMux.Handle(
		RoomserverSetRoomAliasPath,
		httputil.MakeInternalRPCAPI("SetRoomAlias", r.SetRoomAlias),
	)

	internalAPIMux.Handle(
		RoomserverGetRoomIDForAliasPath,
		httputil.MakeInternalRPCAPI("GetRoomIDForAlias", r.GetRoomIDForAlias),
	)

	internalAPIMux.Handle(
		RoomserverGetAliasesForRoomIDPath,
		httputil.MakeInternalRPCAPI("GetAliasesForRoomID", r.GetAliasesForRoomID),
	)

	internalAPIMux.Handle(
		RoomserverRemoveRoomAliasPath,
		httputil.MakeInternalRPCAPI("RemoveRoomAlias", r.RemoveRoomAlias),
	)

	internalAPIMux.Handle(
		RoomserverQueryCurrentStatePath,
		httputil.MakeInternalRPCAPI("QueryCurrentState", r.QueryCurrentState),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomsForUserPath,
		httputil.MakeInternalRPCAPI("QueryRoomsForUser", r.QueryRoomsForUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryBulkStateContentPath,
		httputil.MakeInternalRPCAPI("QueryBulkStateContent", r.QueryBulkStateContent),
	)

	internalAPIMux.Handle(
		RoomserverQuerySharedUsersPath,
		httputil.MakeInternalRPCAPI("QuerySharedUsers", r.QuerySharedUsers),
	)

	internalAPIMux.Handle(
		RoomserverQueryKnownUsersPath,
		httputil.MakeInternalRPCAPI("QueryKnownUsers", r.QueryKnownUsers),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerBannedFromRoomPath,
		httputil.MakeInternalRPCAPI("QueryServerBannedFromRoom", r.QueryServerBannedFromRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryAuthChainPath,
		httputil.MakeInternalRPCAPI("QueryAuthChain", r.QueryAuthChain),
	)

	internalAPIMux.Handle(
		RoomserverQueryRestrictedJoinAllowed,
		httputil.MakeInternalRPCAPI("QueryRestrictedJoinAllowed", r.QueryRestrictedJoinAllowed),
	)
}
