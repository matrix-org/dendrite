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
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
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

	memberEvents, err := r.DB.Events(ctx, memberNIDs)
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
		memberContent.Membership = gomatrixserverlib.Leave

		stateKey := *memberEvent.StateKey()
		fledglingEvent := &gomatrixserverlib.EventBuilder{
			RoomID:     req.RoomID,
			Type:       gomatrixserverlib.MRoomMember,
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

	roomIDs, err := r.DB.GetRoomsByMembership(ctx, req.UserID, gomatrixserverlib.Join)
	if err != nil && err != sql.ErrNoRows {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.GetRoomsByMembership: %s", err),
		}
		return nil
	}

	inviteRoomIDs, err := r.DB.GetRoomsByMembership(ctx, req.UserID, gomatrixserverlib.Invite)
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

		res.Affected = append(res.Affected, roomID)
	}
	return nil
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

	authEventMap := map[string]*gomatrixserverlib.Event{}
	stateEventMap := map[string]*gomatrixserverlib.Event{}

	for _, fwdExtremity := range fwdExtremities {
		var state gomatrixserverlib.RespState
		state, err = r.Inputer.FSAPI.LookupState(ctx, r.Inputer.ServerName, req.ServerName, req.RoomID, fwdExtremity.EventID, roomInfo.RoomVersion)
		if err != nil {
			res.Error = &api.PerformError{
				Code: api.PerformErrorBadRequest,
				Msg:  fmt.Sprintf("r.Inputer.FSAPI.LookupState (%q): %s", fwdExtremity.EventID, err),
			}
			return nil
		}
		for _, authEvent := range state.AuthEvents.UntrustedEvents(roomInfo.RoomVersion) {
			if err = authEvent.VerifyEventSignatures(ctx, r.Inputer.KeyRing); err != nil {
				continue
			}
			authEventMap[authEvent.EventID()] = authEvent
		}
		for _, stateEvent := range state.StateEvents.UntrustedEvents(roomInfo.RoomVersion) {
			if err = stateEvent.VerifyEventSignatures(ctx, r.Inputer.KeyRing); err != nil {
				continue
			}
			stateEventMap[stateEvent.EventID()] = stateEvent
		}
	}

	authEvents := make([]*gomatrixserverlib.HeaderedEvent, 0, len(authEventMap))
	stateEvents := make([]*gomatrixserverlib.HeaderedEvent, 0, len(stateEventMap))
	stateIDs := make([]string, 0, len(stateEventMap))

	for _, authEvent := range authEventMap {
		authEvents = append(authEvents, authEvent.Headered(roomInfo.RoomVersion))
	}
	for _, stateEvent := range stateEventMap {
		stateEvents = append(stateEvents, stateEvent.Headered(roomInfo.RoomVersion))
		stateIDs = append(stateIDs, stateEvent.EventID())
	}

	builder := &gomatrixserverlib.EventBuilder{
		Type:    "org.matrix.dendrite.state_download",
		Sender:  req.UserID,
		RoomID:  req.RoomID,
		Content: gomatrixserverlib.RawJSON("{}"),
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
