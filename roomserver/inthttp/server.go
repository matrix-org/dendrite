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
		httputil.MakeInternalRPCAPI(RoomserverInputRoomEventsPath, r.InputRoomEvents),
	)

	internalAPIMux.Handle(
		RoomserverPerformInvitePath,
		httputil.MakeInternalRPCAPI(RoomserverPerformInvitePath, r.PerformInvite),
	)

	internalAPIMux.Handle(
		RoomserverPerformJoinPath,
		httputil.MakeInternalRPCAPI(RoomserverPerformJoinPath, r.PerformJoin),
	)

	internalAPIMux.Handle(
		RoomserverPerformLeavePath,
		httputil.MakeInternalRPCAPI(RoomserverPerformLeavePath, r.PerformLeave),
	)

	internalAPIMux.Handle(
		RoomserverPerformPeekPath,
		httputil.MakeInternalRPCAPI(RoomserverPerformPeekPath, r.PerformPeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformInboundPeekPath,
		httputil.MakeInternalRPCAPI(RoomserverPerformInboundPeekPath, r.PerformInboundPeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformUnpeekPath,
		httputil.MakeInternalRPCAPI(RoomserverPerformUnpeekPath, r.PerformUnpeek),
	)

	internalAPIMux.Handle(
		RoomserverPerformRoomUpgradePath,
		httputil.MakeInternalRPCAPI(RoomserverPerformRoomUpgradePath, r.PerformRoomUpgrade),
	)

	internalAPIMux.Handle(
		RoomserverPerformPublishPath,
		httputil.MakeInternalRPCAPI(RoomserverPerformPublishPath, r.PerformPublish),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminEvacuateRoomPath,
		httputil.MakeInternalRPCAPI(RoomserverPerformAdminEvacuateRoomPath, r.PerformAdminEvacuateRoom),
	)

	internalAPIMux.Handle(
		RoomserverPerformAdminEvacuateUserPath,
		httputil.MakeInternalRPCAPI(RoomserverPerformAdminEvacuateUserPath, r.PerformAdminEvacuateUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryPublishedRoomsPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryPublishedRoomsPath, r.QueryPublishedRooms),
	)

	internalAPIMux.Handle(
		RoomserverQueryLatestEventsAndStatePath,
		httputil.MakeInternalRPCAPI(RoomserverQueryLatestEventsAndStatePath, r.QueryLatestEventsAndState),
	)

	internalAPIMux.Handle(
		RoomserverQueryStateAfterEventsPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryStateAfterEventsPath, r.QueryStateAfterEvents),
	)

	internalAPIMux.Handle(
		RoomserverQueryEventsByIDPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryEventsByIDPath, r.QueryEventsByID),
	)

	internalAPIMux.Handle(
		RoomserverQueryMembershipForUserPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryMembershipForUserPath, r.QueryMembershipForUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryMembershipsForRoomPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryMembershipsForRoomPath, r.QueryMembershipsForRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerJoinedToRoomPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryServerJoinedToRoomPath, r.QueryServerJoinedToRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerJoinedToRoomPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryServerJoinedToRoomPath, r.QueryServerJoinedToRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerAllowedToSeeEventPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryServerAllowedToSeeEventPath, r.QueryServerAllowedToSeeEvent),
	)

	internalAPIMux.Handle(
		RoomserverQueryMissingEventsPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryMissingEventsPath, r.QueryMissingEvents),
	)

	internalAPIMux.Handle(
		RoomserverQueryStateAndAuthChainPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryStateAndAuthChainPath, r.QueryStateAndAuthChain),
	)

	internalAPIMux.Handle(
		RoomserverPerformBackfillPath,
		httputil.MakeInternalRPCAPI(RoomserverPerformBackfillPath, r.PerformBackfill),
	)

	internalAPIMux.Handle(
		RoomserverPerformForgetPath,
		httputil.MakeInternalRPCAPI(RoomserverPerformForgetPath, r.PerformForget),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomVersionCapabilitiesPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryRoomVersionCapabilitiesPath, r.QueryRoomVersionCapabilities),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomVersionForRoomPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryRoomVersionForRoomPath, r.QueryRoomVersionForRoom),
	)

	internalAPIMux.Handle(
		RoomserverSetRoomAliasPath,
		httputil.MakeInternalRPCAPI(RoomserverSetRoomAliasPath, r.SetRoomAlias),
	)

	internalAPIMux.Handle(
		RoomserverGetRoomIDForAliasPath,
		httputil.MakeInternalRPCAPI(RoomserverGetRoomIDForAliasPath, r.GetRoomIDForAlias),
	)

	internalAPIMux.Handle(
		RoomserverGetAliasesForRoomIDPath,
		httputil.MakeInternalRPCAPI(RoomserverGetAliasesForRoomIDPath, r.GetAliasesForRoomID),
	)

	internalAPIMux.Handle(
		RoomserverRemoveRoomAliasPath,
		httputil.MakeInternalRPCAPI(RoomserverRemoveRoomAliasPath, r.RemoveRoomAlias),
	)

	internalAPIMux.Handle(
		RoomserverQueryCurrentStatePath,
		httputil.MakeInternalRPCAPI(RoomserverQueryCurrentStatePath, r.QueryCurrentState),
	)

	internalAPIMux.Handle(
		RoomserverQueryRoomsForUserPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryRoomsForUserPath, r.QueryRoomsForUser),
	)

	internalAPIMux.Handle(
		RoomserverQueryBulkStateContentPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryBulkStateContentPath, r.QueryBulkStateContent),
	)

	internalAPIMux.Handle(
		RoomserverQuerySharedUsersPath,
		httputil.MakeInternalRPCAPI(RoomserverQuerySharedUsersPath, r.QuerySharedUsers),
	)

	internalAPIMux.Handle(
		RoomserverQueryKnownUsersPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryKnownUsersPath, r.QueryKnownUsers),
	)

	internalAPIMux.Handle(
		RoomserverQueryServerBannedFromRoomPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryServerBannedFromRoomPath, r.QueryServerBannedFromRoom),
	)

	internalAPIMux.Handle(
		RoomserverQueryAuthChainPath,
		httputil.MakeInternalRPCAPI(RoomserverQueryAuthChainPath, r.QueryAuthChain),
	)

	internalAPIMux.Handle(
		RoomserverQueryRestrictedJoinAllowed,
		httputil.MakeInternalRPCAPI(RoomserverQueryRestrictedJoinAllowed, r.QueryRestrictedJoinAllowed),
	)
}
