package api

import (
	"context"
	"encoding/json"
	"fmt"

	asAPI "github.com/matrix-org/dendrite/appservice/api"
	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// RoomserverInternalAPITrace wraps a RoomserverInternalAPI and logs the
// complete request/response/error
type RoomserverInternalAPITrace struct {
	Impl RoomserverInternalAPI
}

func (t *RoomserverInternalAPITrace) SetFederationAPI(fsAPI fsAPI.FederationInternalAPI, keyRing *gomatrixserverlib.KeyRing) {
	t.Impl.SetFederationAPI(fsAPI, keyRing)
}

func (t *RoomserverInternalAPITrace) SetAppserviceAPI(asAPI asAPI.AppServiceQueryAPI) {
	t.Impl.SetAppserviceAPI(asAPI)
}

func (t *RoomserverInternalAPITrace) InputRoomEvents(
	ctx context.Context,
	req *InputRoomEventsRequest,
	res *InputRoomEventsResponse,
) {
	t.Impl.InputRoomEvents(ctx, req, res)
	util.GetLogger(ctx).Infof("InputRoomEvents req=%+v res=%+v", js(req), js(res))
}

func (t *RoomserverInternalAPITrace) PerformInvite(
	ctx context.Context,
	req *PerformInviteRequest,
	res *PerformInviteResponse,
) error {
	util.GetLogger(ctx).Infof("PerformInvite req=%+v res=%+v", js(req), js(res))
	return t.Impl.PerformInvite(ctx, req, res)
}

func (t *RoomserverInternalAPITrace) PerformPeek(
	ctx context.Context,
	req *PerformPeekRequest,
	res *PerformPeekResponse,
) {
	t.Impl.PerformPeek(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformPeek req=%+v res=%+v", js(req), js(res))
}

func (t *RoomserverInternalAPITrace) PerformUnpeek(
	ctx context.Context,
	req *PerformUnpeekRequest,
	res *PerformUnpeekResponse,
) {
	t.Impl.PerformUnpeek(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformUnpeek req=%+v res=%+v", js(req), js(res))
}

func (t *RoomserverInternalAPITrace) PerformJoin(
	ctx context.Context,
	req *PerformJoinRequest,
	res *PerformJoinResponse,
) {
	t.Impl.PerformJoin(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformJoin req=%+v res=%+v", js(req), js(res))
}

func (t *RoomserverInternalAPITrace) PerformLeave(
	ctx context.Context,
	req *PerformLeaveRequest,
	res *PerformLeaveResponse,
) error {
	err := t.Impl.PerformLeave(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("PerformLeave req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) PerformPublish(
	ctx context.Context,
	req *PerformPublishRequest,
	res *PerformPublishResponse,
) {
	t.Impl.PerformPublish(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformPublish req=%+v res=%+v", js(req), js(res))
}

func (t *RoomserverInternalAPITrace) PerformInboundPeek(
	ctx context.Context,
	req *PerformInboundPeekRequest,
	res *PerformInboundPeekResponse,
) error {
	err := t.Impl.PerformInboundPeek(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformInboundPeek req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryPublishedRooms(
	ctx context.Context,
	req *QueryPublishedRoomsRequest,
	res *QueryPublishedRoomsResponse,
) error {
	err := t.Impl.QueryPublishedRooms(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryPublishedRooms req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryLatestEventsAndState(
	ctx context.Context,
	req *QueryLatestEventsAndStateRequest,
	res *QueryLatestEventsAndStateResponse,
) error {
	err := t.Impl.QueryLatestEventsAndState(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryLatestEventsAndState req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryStateAfterEvents(
	ctx context.Context,
	req *QueryStateAfterEventsRequest,
	res *QueryStateAfterEventsResponse,
) error {
	err := t.Impl.QueryStateAfterEvents(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryStateAfterEvents req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryMissingAuthPrevEvents(
	ctx context.Context,
	req *QueryMissingAuthPrevEventsRequest,
	res *QueryMissingAuthPrevEventsResponse,
) error {
	err := t.Impl.QueryMissingAuthPrevEvents(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryMissingAuthPrevEvents req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryEventsByID(
	ctx context.Context,
	req *QueryEventsByIDRequest,
	res *QueryEventsByIDResponse,
) error {
	err := t.Impl.QueryEventsByID(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryEventsByID req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryMembershipForUser(
	ctx context.Context,
	req *QueryMembershipForUserRequest,
	res *QueryMembershipForUserResponse,
) error {
	err := t.Impl.QueryMembershipForUser(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryMembershipForUser req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryMembershipsForRoom(
	ctx context.Context,
	req *QueryMembershipsForRoomRequest,
	res *QueryMembershipsForRoomResponse,
) error {
	err := t.Impl.QueryMembershipsForRoom(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryMembershipsForRoom req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryServerJoinedToRoom(
	ctx context.Context,
	req *QueryServerJoinedToRoomRequest,
	res *QueryServerJoinedToRoomResponse,
) error {
	err := t.Impl.QueryServerJoinedToRoom(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryServerJoinedToRoom req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryServerAllowedToSeeEvent(
	ctx context.Context,
	req *QueryServerAllowedToSeeEventRequest,
	res *QueryServerAllowedToSeeEventResponse,
) error {
	err := t.Impl.QueryServerAllowedToSeeEvent(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryServerAllowedToSeeEvent req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryMissingEvents(
	ctx context.Context,
	req *QueryMissingEventsRequest,
	res *QueryMissingEventsResponse,
) error {
	err := t.Impl.QueryMissingEvents(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryMissingEvents req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryStateAndAuthChain(
	ctx context.Context,
	req *QueryStateAndAuthChainRequest,
	res *QueryStateAndAuthChainResponse,
) error {
	err := t.Impl.QueryStateAndAuthChain(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryStateAndAuthChain req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) PerformBackfill(
	ctx context.Context,
	req *PerformBackfillRequest,
	res *PerformBackfillResponse,
) error {
	err := t.Impl.PerformBackfill(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("PerformBackfill req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) PerformForget(
	ctx context.Context,
	req *PerformForgetRequest,
	res *PerformForgetResponse,
) error {
	err := t.Impl.PerformForget(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("PerformForget req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryRoomVersionCapabilities(
	ctx context.Context,
	req *QueryRoomVersionCapabilitiesRequest,
	res *QueryRoomVersionCapabilitiesResponse,
) error {
	err := t.Impl.QueryRoomVersionCapabilities(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryRoomVersionCapabilities req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryRoomVersionForRoom(
	ctx context.Context,
	req *QueryRoomVersionForRoomRequest,
	res *QueryRoomVersionForRoomResponse,
) error {
	err := t.Impl.QueryRoomVersionForRoom(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryRoomVersionForRoom req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) SetRoomAlias(
	ctx context.Context,
	req *SetRoomAliasRequest,
	res *SetRoomAliasResponse,
) error {
	err := t.Impl.SetRoomAlias(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("SetRoomAlias req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) GetRoomIDForAlias(
	ctx context.Context,
	req *GetRoomIDForAliasRequest,
	res *GetRoomIDForAliasResponse,
) error {
	err := t.Impl.GetRoomIDForAlias(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("GetRoomIDForAlias req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) GetAliasesForRoomID(
	ctx context.Context,
	req *GetAliasesForRoomIDRequest,
	res *GetAliasesForRoomIDResponse,
) error {
	err := t.Impl.GetAliasesForRoomID(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("GetAliasesForRoomID req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) GetCreatorIDForAlias(
	ctx context.Context,
	req *GetCreatorIDForAliasRequest,
	res *GetCreatorIDForAliasResponse,
) error {
	err := t.Impl.GetCreatorIDForAlias(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("GetCreatorIDForAlias req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) RemoveRoomAlias(
	ctx context.Context,
	req *RemoveRoomAliasRequest,
	res *RemoveRoomAliasResponse,
) error {
	err := t.Impl.RemoveRoomAlias(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("RemoveRoomAlias req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryCurrentState(ctx context.Context, req *QueryCurrentStateRequest, res *QueryCurrentStateResponse) error {
	err := t.Impl.QueryCurrentState(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryCurrentState req=%+v res=%+v", js(req), js(res))
	return err
}

// QueryRoomsForUser retrieves a list of room IDs matching the given query.
func (t *RoomserverInternalAPITrace) QueryRoomsForUser(ctx context.Context, req *QueryRoomsForUserRequest, res *QueryRoomsForUserResponse) error {
	err := t.Impl.QueryRoomsForUser(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryRoomsForUser req=%+v res=%+v", js(req), js(res))
	return err
}

// QueryBulkStateContent does a bulk query for state event content in the given rooms.
func (t *RoomserverInternalAPITrace) QueryBulkStateContent(ctx context.Context, req *QueryBulkStateContentRequest, res *QueryBulkStateContentResponse) error {
	err := t.Impl.QueryBulkStateContent(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryBulkStateContent req=%+v res=%+v", js(req), js(res))
	return err
}

// QuerySharedUsers returns a list of users who share at least 1 room in common with the given user.
func (t *RoomserverInternalAPITrace) QuerySharedUsers(ctx context.Context, req *QuerySharedUsersRequest, res *QuerySharedUsersResponse) error {
	err := t.Impl.QuerySharedUsers(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QuerySharedUsers req=%+v res=%+v", js(req), js(res))
	return err
}

// QueryKnownUsers returns a list of users that we know about from our joined rooms.
func (t *RoomserverInternalAPITrace) QueryKnownUsers(ctx context.Context, req *QueryKnownUsersRequest, res *QueryKnownUsersResponse) error {
	err := t.Impl.QueryKnownUsers(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryKnownUsers req=%+v res=%+v", js(req), js(res))
	return err
}

// QueryServerBannedFromRoom returns whether a server is banned from a room by server ACLs.
func (t *RoomserverInternalAPITrace) QueryServerBannedFromRoom(ctx context.Context, req *QueryServerBannedFromRoomRequest, res *QueryServerBannedFromRoomResponse) error {
	err := t.Impl.QueryServerBannedFromRoom(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("QueryServerBannedFromRoom req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) QueryAuthChain(
	ctx context.Context,
	request *QueryAuthChainRequest,
	response *QueryAuthChainResponse,
) error {
	err := t.Impl.QueryAuthChain(ctx, request, response)
	util.GetLogger(ctx).WithError(err).Infof("QueryAuthChain req=%+v res=%+v", js(request), js(response))
	return err
}

func js(thing interface{}) string {
	b, err := json.Marshal(thing)
	if err != nil {
		return fmt.Sprintf("Marshal error:%s", err)
	}
	return string(b)
}
