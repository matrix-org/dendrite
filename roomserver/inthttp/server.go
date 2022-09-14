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
		httputil.MakeInternalRPCAPI("RoomserverInputRoomEvents", r.InputRoomEvents),
	)

	internalAPIMux.Handle(
		RoomserverPerformInvitePath,
		httputil.MakeInternalRPCAPI("RoomserverPerformInvite", r.PerformInvite),
	)

	internalAPIMux.Handle(
		RoomserverPerformJoinPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformJoin", r.PerformJoin),
	)

	internalAPIMux.Handle(
		RoomserverPerformLeavePath,
		httputil.MakeInternalRPCAPI("RoomserverPerformLeave", r.PerformLeave),
	)

	internalAPIMux.Handle(
		RoomserverPerformPeekPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformPeek", r.PerformPeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformInboundPeekPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformInboundPeek", r.PerformInboundPeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformUnpeekPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformUnpeek", r.PerformUnpeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformRoomUpgradePath,
		httputil.MakeInternalRPCAPI("RoomserverPerformRoomUpgrade", r.PerformRoomUpgrade),
	)

	internalAPIMux.Handle(
		RoomserverPerformPublishPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformPublish", r.PerformPublish),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminEvacuateRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformAdminEvacuateRoom", r.PerformAdminEvacuateRoom),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminEvacuateUserPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformAdminEvacuateUser", r.PerformAdminEvacuateUser),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminDownloadStatePath,
		httputil.MakeInternalRPCAPI("RoomserverPerformAdminDownloadState", r.PerformAdminDownloadState),
	)

	internalAPIMux.Handle(
		RoomserverQueryPublishedRoomsPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryPublishedRooms", r.QueryPublishedRooms),
	)

	internalAPIMux.Handle(
		RoomserverQueryLatestEventsAndStatePath,
		httputil.MakeInternalRPCAPI("RoomserverQueryLatestEventsAndState", r.QueryLatestEventsAndState),
	)

	internalAPIMux.Handle(
		RoomserverQueryStateAfterEventsPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryStateAfterEvents", r.QueryStateAfterEvents),
	)

	internalAPIMux.Handle(
		RoomserverQueryEventsByIDPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryEventsByID", r.QueryEventsByID),
	)

	internalAPIMux.Handle(
		RoomserverQueryMembershipForUserPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryMembershipForUser", r.QueryMembershipForUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryMembershipsForRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryMembershipsForRoom", r.QueryMembershipsForRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerJoinedToRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryServerJoinedToRoom", r.QueryServerJoinedToRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerAllowedToSeeEventPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryServerAllowedToSeeEvent", r.QueryServerAllowedToSeeEvent),
	)

	internalAPIMux.Handle(
		RoomserverQueryMissingEventsPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryMissingEvents", r.QueryMissingEvents),
	)

	internalAPIMux.Handle(
		RoomserverQueryStateAndAuthChainPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryStateAndAuthChain", r.QueryStateAndAuthChain),
	)

	internalAPIMux.Handle(
		RoomserverPerformBackfillPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformBackfill", r.PerformBackfill),
	)

	internalAPIMux.Handle(
		RoomserverPerformForgetPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformForget", r.PerformForget),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomVersionCapabilitiesPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryRoomVersionCapabilities", r.QueryRoomVersionCapabilities),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomVersionForRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryRoomVersionForRoom", r.QueryRoomVersionForRoom),
	)

	internalAPIMux.Handle(
		RoomserverSetRoomAliasPath,
		httputil.MakeInternalRPCAPI("RoomserverSetRoomAlias", r.SetRoomAlias),
	)

	internalAPIMux.Handle(
		RoomserverGetRoomIDForAliasPath,
		httputil.MakeInternalRPCAPI("RoomserverGetRoomIDForAlias", r.GetRoomIDForAlias),
	)

	internalAPIMux.Handle(
		RoomserverGetAliasesForRoomIDPath,
		httputil.MakeInternalRPCAPI("RoomserverGetAliasesForRoomID", r.GetAliasesForRoomID),
	)

	internalAPIMux.Handle(
		RoomserverRemoveRoomAliasPath,
		httputil.MakeInternalRPCAPI("RoomserverRemoveRoomAlias", r.RemoveRoomAlias),
	)

	internalAPIMux.Handle(
		RoomserverQueryCurrentStatePath,
		httputil.MakeInternalRPCAPI("RoomserverQueryCurrentState", r.QueryCurrentState),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomsForUserPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryRoomsForUser", r.QueryRoomsForUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryBulkStateContentPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryBulkStateContent", r.QueryBulkStateContent),
	)

	internalAPIMux.Handle(
		RoomserverQuerySharedUsersPath,
		httputil.MakeInternalRPCAPI("RoomserverQuerySharedUsers", r.QuerySharedUsers),
	)

	internalAPIMux.Handle(
		RoomserverQueryKnownUsersPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryKnownUsers", r.QueryKnownUsers),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerBannedFromRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryServerBannedFromRoom", r.QueryServerBannedFromRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryAuthChainPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryAuthChain", r.QueryAuthChain),
	)

	internalAPIMux.Handle(
		RoomserverQueryRestrictedJoinAllowed,
		httputil.MakeInternalRPCAPI("RoomserverQueryRestrictedJoinAllowed", r.QueryRestrictedJoinAllowed),
	)
	internalAPIMux.Handle(
		RoomserverQueryMembershipAtEventPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryMembershipAtEventPath", r.QueryMembershipAtEvent),
	)
}
