// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package perform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/internal/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"
)

type Admin struct {
	DB      storage.Database
	Cfg     *config.RoomServer
	Queryer *query.Queryer
	Inputer *input.Inputer
	Leaver  *Leaver
}

// PerformEvacuateRoom will remove all local users from the given room.
func (r *Admin) PerformAdminEvacuateRoom(
	ctx context.Context,
	req *api.PerformAdminEvacuateRoomRequest,
	res *api.PerformAdminEvacuateRoomResponse,
) error {
	roomInfo, err := r.DB.RoomInfo(ctx, req.RoomID)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.RoomInfo: %s", err),
		}
		return nil
	}
	if roomInfo == nil || roomInfo.IsStub() {
		res.Error = &api.PerformError{
			Code: api.PerformErrorNoRoom,
			Msg:  fmt.Sprintf("Room %s not found", req.RoomID),
		}
		return nil
	}

	memberNIDs, err := r.DB.GetMembershipEventNIDsForRoom(ctx, roomInfo.RoomNID, true, true)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.GetMembershipEventNIDsForRoom: %s", err),
		}
		return nil
	}

	memberEvents, err := r.DB.Events(ctx, roomInfo, memberNIDs)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.Events: %s", err),
		}
		return nil
	}

	inputEvents := make([]api.InputRoomEvent, 0, len(memberEvents))
	res.Affected = make([]string, 0, len(memberEvents))
	latestReq := &api.QueryLatestEventsAndStateRequest{
		RoomID: req.RoomID,
	}
	latestRes := &api.QueryLatestEventsAndStateResponse{}
	if err = r.Queryer.QueryLatestEventsAndState(ctx, latestReq, latestRes); err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.Queryer.QueryLatestEventsAndState: %s", err),
		}
		return nil
	}

	prevEvents := latestRes.LatestEvents
	for _, memberEvent := range memberEvents {
		if memberEvent.StateKey() == nil {
			continue
		}

		var memberContent gomatrixserverlib.MemberContent
		if err = json.Unmarshal(memberEvent.Content(), &memberContent); err != nil {
			res.Error = &api.PerformError{
				Code: api.PerformErrorBadRequest,
				Msg:  fmt.Sprintf("json.Unmarshal: %s", err),
			}
			return nil
		}
		memberContent.Membership = spec.Leave

		stateKey := *memberEvent.StateKey()
		fledglingEvent := &gomatrixserverlib.EventBuilder{
			RoomID:     req.RoomID,
			Type:       spec.MRoomMember,
			StateKey:   &stateKey,
			Sender:     stateKey,
			PrevEvents: prevEvents,
		}

		_, senderDomain, err := gomatrixserverlib.SplitID('@', fledglingEvent.Sender)
		if err != nil {
			continue
		}

		if fledglingEvent.Content, err = json.Marshal(memberContent); err != nil {
			res.Error = &api.PerformError{
				Code: api.PerformErrorBadRequest,
				Msg:  fmt.Sprintf("json.Marshal: %s", err),
			}
			return nil
		}

		eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(fledglingEvent)
		if err != nil {
			res.Error = &api.PerformError{
				Code: api.PerformErrorBadRequest,
				Msg:  fmt.Sprintf("gomatrixserverlib.StateNeededForEventBuilder: %s", err),
			}
			return nil
		}

		identity, err := r.Cfg.Matrix.SigningIdentityFor(senderDomain)
		if err != nil {
			continue
		}

		event, err := eventutil.BuildEvent(ctx, fledglingEvent, r.Cfg.Matrix, identity, time.Now(), &eventsNeeded, latestRes)
		if err != nil {
			res.Error = &api.PerformError{
				Code: api.PerformErrorBadRequest,
				Msg:  fmt.Sprintf("eventutil.BuildEvent: %s", err),
			}
			return nil
		}

		inputEvents = append(inputEvents, api.InputRoomEvent{
			Kind:         api.KindNew,
			Event:        event,
			Origin:       senderDomain,
			SendAsServer: string(senderDomain),
		})
		res.Affected = append(res.Affected, stateKey)
		prevEvents = []gomatrixserverlib.EventReference{
			event.EventReference(),
		}
	}

	inputReq := &api.InputRoomEventsRequest{
		InputRoomEvents: inputEvents,
		Asynchronous:    true,
	}
	inputRes := &api.InputRoomEventsResponse{}
	return r.Inputer.InputRoomEvents(ctx, inputReq, inputRes)
}

func (r *Admin) PerformAdminEvacuateUser(
	ctx context.Context,
	req *api.PerformAdminEvacuateUserRequest,
	res *api.PerformAdminEvacuateUserResponse,
) error {
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Malformed user ID: %s", err),
		}
		return nil
	}
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  "Can only evacuate local users using this endpoint",
		}
		return nil
	}

	roomIDs, err := r.DB.GetRoomsByMembership(ctx, req.UserID, spec.Join)
	if err != nil && err != sql.ErrNoRows {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.GetRoomsByMembership: %s", err),
		}
		return nil
	}

	inviteRoomIDs, err := r.DB.GetRoomsByMembership(ctx, req.UserID, spec.Invite)
	if err != nil && err != sql.ErrNoRows {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.GetRoomsByMembership: %s", err),
		}
		return nil
	}

	for _, roomID := range append(roomIDs, inviteRoomIDs...) {
		leaveReq := &api.PerformLeaveRequest{
			RoomID: roomID,
			UserID: req.UserID,
		}
		leaveRes := &api.PerformLeaveResponse{}
		outputEvents, err := r.Leaver.PerformLeave(ctx, leaveReq, leaveRes)
		if err != nil {
			res.Error = &api.PerformError{
				Code: api.PerformErrorBadRequest,
				Msg:  fmt.Sprintf("r.Leaver.PerformLeave: %s", err),
			}
			return nil
		}
		res.Affected = append(res.Affected, roomID)
		if len(outputEvents) == 0 {
			continue
		}
		if err := r.Inputer.OutputProducer.ProduceRoomEvents(roomID, outputEvents); err != nil {
			res.Error = &api.PerformError{
				Code: api.PerformErrorBadRequest,
				Msg:  fmt.Sprintf("r.Inputer.WriteOutputEvents: %s", err),
			}
			return nil
		}
	}
	return nil
}

func (r *Admin) PerformAdminPurgeRoom(
	ctx context.Context,
	req *api.PerformAdminPurgeRoomRequest,
	res *api.PerformAdminPurgeRoomResponse,
) error {
	// Validate we actually got a room ID and nothing else
	if _, _, err := gomatrixserverlib.SplitID('!', req.RoomID); err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Malformed room ID: %s", err),
		}
		return nil
	}

	logrus.WithField("room_id", req.RoomID).Warn("Purging room from roomserver")
	if err := r.DB.PurgeRoom(ctx, req.RoomID); err != nil {
		logrus.WithField("room_id", req.RoomID).WithError(err).Warn("Failed to purge room from roomserver")
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  err.Error(),
		}
		return nil
	}

	logrus.WithField("room_id", req.RoomID).Warn("Room purged from roomserver")

	return r.Inputer.OutputProducer.ProduceRoomEvents(req.RoomID, []api.OutputEvent{
		{
			Type: api.OutputTypePurgeRoom,
			PurgeRoom: &api.OutputPurgeRoom{
				RoomID: req.RoomID,
			},
		},
	})
}

func (r *Admin) PerformAdminDownloadState(
	ctx context.Context,
	req *api.PerformAdminDownloadStateRequest,
	res *api.PerformAdminDownloadStateResponse,
) error {
	_, senderDomain, err := r.Cfg.Matrix.SplitLocalID('@', req.UserID)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.Cfg.Matrix.SplitLocalID: %s", err),
		}
		return nil
	}

	roomInfo, err := r.DB.RoomInfo(ctx, req.RoomID)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.RoomInfo: %s", err),
		}
		return nil
	}

	if roomInfo == nil || roomInfo.IsStub() {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("room %q not found", req.RoomID),
		}
		return nil
	}

	fwdExtremities, _, depth, err := r.DB.LatestEventIDs(ctx, roomInfo.RoomNID)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.LatestEventIDs: %s", err),
		}
		return nil
	}

	authEventMap := map[string]gomatrixserverlib.PDU{}
	stateEventMap := map[string]gomatrixserverlib.PDU{}

	for _, fwdExtremity := range fwdExtremities {
		var state gomatrixserverlib.StateResponse
		state, err = r.Inputer.FSAPI.LookupState(ctx, r.Inputer.ServerName, req.ServerName, req.RoomID, fwdExtremity.EventID, roomInfo.RoomVersion)
		if err != nil {
			res.Error = &api.PerformError{
				Code: api.PerformErrorBadRequest,
				Msg:  fmt.Sprintf("r.Inputer.FSAPI.LookupState (%q): %s", fwdExtremity.EventID, err),
			}
			return nil
		}
		for _, authEvent := range state.GetAuthEvents().UntrustedEvents(roomInfo.RoomVersion) {
			if err = gomatrixserverlib.VerifyEventSignatures(ctx, authEvent, r.Inputer.KeyRing); err != nil {
				continue
			}
			authEventMap[authEvent.EventID()] = authEvent
		}
		for _, stateEvent := range state.GetStateEvents().UntrustedEvents(roomInfo.RoomVersion) {
			if err = gomatrixserverlib.VerifyEventSignatures(ctx, stateEvent, r.Inputer.KeyRing); err != nil {
				continue
			}
			stateEventMap[stateEvent.EventID()] = stateEvent
		}
	}

	authEvents := make([]*types.HeaderedEvent, 0, len(authEventMap))
	stateEvents := make([]*types.HeaderedEvent, 0, len(stateEventMap))
	stateIDs := make([]string, 0, len(stateEventMap))

	for _, authEvent := range authEventMap {
		authEvents = append(authEvents, &types.HeaderedEvent{PDU: authEvent})
	}
	for _, stateEvent := range stateEventMap {
		stateEvents = append(stateEvents, &types.HeaderedEvent{PDU: stateEvent})
		stateIDs = append(stateIDs, stateEvent.EventID())
	}

	builder := &gomatrixserverlib.EventBuilder{
		Type:    "org.matrix.dendrite.state_download",
		Sender:  req.UserID,
		RoomID:  req.RoomID,
		Content: spec.RawJSON("{}"),
	}

	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("gomatrixserverlib.StateNeededForEventBuilder: %s", err),
		}
		return nil
	}

	queryRes := &api.QueryLatestEventsAndStateResponse{
		RoomExists:   true,
		RoomVersion:  roomInfo.RoomVersion,
		LatestEvents: fwdExtremities,
		StateEvents:  stateEvents,
		Depth:        depth,
	}

	identity, err := r.Cfg.Matrix.SigningIdentityFor(senderDomain)
	if err != nil {
		return err
	}

	ev, err := eventutil.BuildEvent(ctx, builder, r.Cfg.Matrix, identity, time.Now(), &eventsNeeded, queryRes)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("eventutil.BuildEvent: %s", err),
		}
		return nil
	}

	inputReq := &api.InputRoomEventsRequest{
		Asynchronous: false,
	}
	inputRes := &api.InputRoomEventsResponse{}

	for _, authEvent := range append(authEvents, stateEvents...) {
		inputReq.InputRoomEvents = append(inputReq.InputRoomEvents, api.InputRoomEvent{
			Kind:  api.KindOutlier,
			Event: authEvent,
		})
	}

	inputReq.InputRoomEvents = append(inputReq.InputRoomEvents, api.InputRoomEvent{
		Kind:          api.KindNew,
		Event:         ev,
		Origin:        r.Cfg.Matrix.ServerName,
		HasState:      true,
		StateEventIDs: stateIDs,
		SendAsServer:  string(r.Cfg.Matrix.ServerName),
	})

	if err := r.Inputer.InputRoomEvents(ctx, inputReq, inputRes); err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.Inputer.InputRoomEvents: %s", err),
		}
		return nil
	}

	if inputRes.ErrMsg != "" {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  inputRes.ErrMsg,
		}
	}

	return nil
}
