package api

import (
	"context"
	"encoding/json"
	"fmt"

	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/util"
)

// RoomserverInternalAPITrace wraps a RoomserverInternalAPI and logs the
// complete request/response/error
type RoomserverInternalAPITrace struct {
	Impl RoomserverInternalAPI
}

func (t *RoomserverInternalAPITrace) SetFederationSenderAPI(fsAPI fsAPI.FederationSenderInternalAPI) {
	t.Impl.SetFederationSenderAPI(fsAPI)
}

func (t *RoomserverInternalAPITrace) InputRoomEvents(
	ctx context.Context,
	req *InputRoomEventsRequest,
	res *InputRoomEventsResponse,
) error {
	err := t.Impl.InputRoomEvents(ctx, req, res)
	util.GetLogger(ctx).WithError(err).Infof("InputRoomEvents req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *RoomserverInternalAPITrace) PerformInvite(
	ctx context.Context,
	req *PerformInviteRequest,
	res *PerformInviteResponse,
) error {
	util.GetLogger(ctx).Infof("PerformInvite req=%+v res=%+v", js(req), js(res))
	return t.Impl.PerformInvite(ctx, req, res)
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

func js(thing interface{}) string {
	b, err := json.Marshal(thing)
	if err != nil {
		return fmt.Sprintf("Marshal error:%s", err)
	}
	return string(b)
}
