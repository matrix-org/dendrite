package inthttp

import (
	"github.com/gorilla/mux"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
)

// AddRoutes adds the RoomserverInternalAPI handlers to the http.ServeMux.
// nolint: gocyclo
func AddRoutes(r api.RoomserverInternalAPI, internalAPIMux *mux.Router, enableMetrics bool) {
	internalAPIMux.Handle(
		RoomserverInputRoomEventsPath,
		httputil.MakeInternalRPCAPI("RoomserverInputRoomEvents", enableMetrics, r.InputRoomEvents),
	)

	internalAPIMux.Handle(
		RoomserverPerformInvitePath,
		httputil.MakeInternalRPCAPI("RoomserverPerformInvite", enableMetrics, r.PerformInvite),
	)

	internalAPIMux.Handle(
		RoomserverPerformJoinPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformJoin", enableMetrics, r.PerformJoin),
	)

	internalAPIMux.Handle(
		RoomserverPerformLeavePath,
		httputil.MakeInternalRPCAPI("RoomserverPerformLeave", enableMetrics, r.PerformLeave),
	)

	internalAPIMux.Handle(
		RoomserverPerformPeekPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformPeek", enableMetrics, r.PerformPeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformInboundPeekPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformInboundPeek", enableMetrics, r.PerformInboundPeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformUnpeekPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformUnpeek", enableMetrics, r.PerformUnpeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformRoomUpgradePath,
		httputil.MakeInternalRPCAPI("RoomserverPerformRoomUpgrade", enableMetrics, r.PerformRoomUpgrade),
	)

	internalAPIMux.Handle(
		RoomserverPerformPublishPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformPublish", enableMetrics, r.PerformPublish),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminEvacuateRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformAdminEvacuateRoom", enableMetrics, r.PerformAdminEvacuateRoom),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminEvacuateUserPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformAdminEvacuateUser", enableMetrics, r.PerformAdminEvacuateUser),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminDownloadStatePath,
		httputil.MakeInternalRPCAPI("RoomserverPerformAdminDownloadState", enableMetrics, r.PerformAdminDownloadState),
	)

	internalAPIMux.Handle(
		RoomserverQueryPublishedRoomsPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryPublishedRooms", enableMetrics, r.QueryPublishedRooms),
	)

	internalAPIMux.Handle(
		RoomserverQueryLatestEventsAndStatePath,
		httputil.MakeInternalRPCAPI("RoomserverQueryLatestEventsAndState", enableMetrics, r.QueryLatestEventsAndState),
	)

	internalAPIMux.Handle(
		RoomserverQueryStateAfterEventsPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryStateAfterEvents", enableMetrics, r.QueryStateAfterEvents),
	)

	internalAPIMux.Handle(
		RoomserverQueryEventsByIDPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryEventsByID", enableMetrics, r.QueryEventsByID),
	)

	internalAPIMux.Handle(
		RoomserverQueryMembershipForUserPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryMembershipForUser", enableMetrics, r.QueryMembershipForUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryMembershipsForRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryMembershipsForRoom", enableMetrics, r.QueryMembershipsForRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerJoinedToRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryServerJoinedToRoom", enableMetrics, r.QueryServerJoinedToRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerAllowedToSeeEventPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryServerAllowedToSeeEvent", enableMetrics, r.QueryServerAllowedToSeeEvent),
	)

	internalAPIMux.Handle(
		RoomserverQueryMissingEventsPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryMissingEvents", enableMetrics, r.QueryMissingEvents),
	)

	internalAPIMux.Handle(
		RoomserverQueryStateAndAuthChainPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryStateAndAuthChain", enableMetrics, r.QueryStateAndAuthChain),
	)

	internalAPIMux.Handle(
		RoomserverPerformBackfillPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformBackfill", enableMetrics, r.PerformBackfill),
	)

	internalAPIMux.Handle(
		RoomserverPerformForgetPath,
		httputil.MakeInternalRPCAPI("RoomserverPerformForget", enableMetrics, r.PerformForget),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomVersionCapabilitiesPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryRoomVersionCapabilities", enableMetrics, r.QueryRoomVersionCapabilities),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomVersionForRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryRoomVersionForRoom", enableMetrics, r.QueryRoomVersionForRoom),
	)

	internalAPIMux.Handle(
		RoomserverSetRoomAliasPath,
		httputil.MakeInternalRPCAPI("RoomserverSetRoomAlias", enableMetrics, r.SetRoomAlias),
	)

	internalAPIMux.Handle(
		RoomserverGetRoomIDForAliasPath,
		httputil.MakeInternalRPCAPI("RoomserverGetRoomIDForAlias", enableMetrics, r.GetRoomIDForAlias),
	)

	internalAPIMux.Handle(
		RoomserverGetAliasesForRoomIDPath,
		httputil.MakeInternalRPCAPI("RoomserverGetAliasesForRoomID", enableMetrics, r.GetAliasesForRoomID),
	)

	internalAPIMux.Handle(
		RoomserverRemoveRoomAliasPath,
		httputil.MakeInternalRPCAPI("RoomserverRemoveRoomAlias", enableMetrics, r.RemoveRoomAlias),
	)

	internalAPIMux.Handle(
		RoomserverQueryCurrentStatePath,
		httputil.MakeInternalRPCAPI("RoomserverQueryCurrentState", enableMetrics, r.QueryCurrentState),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomsForUserPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryRoomsForUser", enableMetrics, r.QueryRoomsForUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryBulkStateContentPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryBulkStateContent", enableMetrics, r.QueryBulkStateContent),
	)

	internalAPIMux.Handle(
		RoomserverQuerySharedUsersPath,
		httputil.MakeInternalRPCAPI("RoomserverQuerySharedUsers", enableMetrics, r.QuerySharedUsers),
	)

	internalAPIMux.Handle(
		RoomserverQueryKnownUsersPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryKnownUsers", enableMetrics, r.QueryKnownUsers),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerBannedFromRoomPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryServerBannedFromRoom", enableMetrics, r.QueryServerBannedFromRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryAuthChainPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryAuthChain", enableMetrics, r.QueryAuthChain),
	)

	internalAPIMux.Handle(
		RoomserverQueryRestrictedJoinAllowed,
		httputil.MakeInternalRPCAPI("RoomserverQueryRestrictedJoinAllowed", enableMetrics, r.QueryRestrictedJoinAllowed),
	)
	internalAPIMux.Handle(
		RoomserverQueryMembershipAtEventPath,
		httputil.MakeInternalRPCAPI("RoomserverQueryMembershipAtEventPath", enableMetrics, r.QueryMembershipAtEvent),
	)
}
