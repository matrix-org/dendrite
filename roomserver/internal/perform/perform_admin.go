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
	"github.com/matrix-org/gomatrixserverlib/fclient"
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

// PerformAdminEvacuateRoom will remove all local users from the given room.
func (r *Admin) PerformAdminEvacuateRoom(
	ctx context.Context,
	roomID string,
) (affected []string, err error) {
	roomInfo, err := r.DB.RoomInfo(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if roomInfo == nil || roomInfo.IsStub() {
		return nil, eventutil.ErrRoomNoExists{}
	}

	memberNIDs, err := r.DB.GetMembershipEventNIDsForRoom(ctx, roomInfo.RoomNID, true, true)
	if err != nil {
		return nil, err
	}

	memberEvents, err := r.DB.Events(ctx, roomInfo.RoomVersion, memberNIDs)
	if err != nil {
		return nil, err
	}

	inputEvents := make([]api.InputRoomEvent, 0, len(memberEvents))
	affected = make([]string, 0, len(memberEvents))
	latestReq := &api.QueryLatestEventsAndStateRequest{
		RoomID: roomID,
	}
	latestRes := &api.QueryLatestEventsAndStateResponse{}
	if err = r.Queryer.QueryLatestEventsAndState(ctx, latestReq, latestRes); err != nil {
		return nil, err
	}

	prevEvents := latestRes.LatestEvents
	var senderDomain spec.ServerName
	var eventsNeeded gomatrixserverlib.StateNeeded
	var identity *fclient.SigningIdentity
	var event *types.HeaderedEvent
	for _, memberEvent := range memberEvents {
		if memberEvent.StateKey() == nil {
			continue
		}

		var memberContent gomatrixserverlib.MemberContent
		if err = json.Unmarshal(memberEvent.Content(), &memberContent); err != nil {
			return nil, err
		}
		memberContent.Membership = spec.Leave

		stateKey := *memberEvent.StateKey()
		fledglingEvent := &gomatrixserverlib.ProtoEvent{
			RoomID:     roomID,
			Type:       spec.MRoomMember,
			StateKey:   &stateKey,
			SenderID:   stateKey,
			PrevEvents: prevEvents,
		}

		userID, err := r.Queryer.QueryUserIDForSender(ctx, roomID, spec.SenderID(fledglingEvent.SenderID))
		if err != nil || userID == nil {
			continue
		}
		senderDomain = userID.Domain()

		if fledglingEvent.Content, err = json.Marshal(memberContent); err != nil {
			return nil, err
		}

		eventsNeeded, err = gomatrixserverlib.StateNeededForProtoEvent(fledglingEvent)
		if err != nil {
			return nil, err
		}

		identity, err = r.Cfg.Matrix.SigningIdentityFor(senderDomain)
		if err != nil {
			continue
		}

		event, err = eventutil.BuildEvent(ctx, fledglingEvent, identity, time.Now(), &eventsNeeded, latestRes)
		if err != nil {
			return nil, err
		}

		inputEvents = append(inputEvents, api.InputRoomEvent{
			Kind:         api.KindNew,
			Event:        event,
			Origin:       senderDomain,
			SendAsServer: string(senderDomain),
		})
		affected = append(affected, stateKey)
		prevEvents = []string{event.EventID()}
	}

	inputReq := &api.InputRoomEventsRequest{
		InputRoomEvents: inputEvents,
		Asynchronous:    false,
	}
	inputRes := &api.InputRoomEventsResponse{}
	r.Inputer.InputRoomEvents(ctx, inputReq, inputRes)
	return affected, nil
}

// PerformAdminEvacuateUser will remove the given user from all rooms.
func (r *Admin) PerformAdminEvacuateUser(
	ctx context.Context,
	userID string,
) (affected []string, err error) {
	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		return nil, fmt.Errorf("can only evacuate local users using this endpoint")
	}

	roomIDs, err := r.DB.GetRoomsByMembership(ctx, userID, spec.Join)
	if err != nil {
		return nil, err
	}

	inviteRoomIDs, err := r.DB.GetRoomsByMembership(ctx, userID, spec.Invite)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	allRooms := append(roomIDs, inviteRoomIDs...)
	affected = make([]string, 0, len(allRooms))
	for _, roomID := range allRooms {
		leaveReq := &api.PerformLeaveRequest{
			RoomID: roomID,
			UserID: userID,
		}
		leaveRes := &api.PerformLeaveResponse{}
		outputEvents, err := r.Leaver.PerformLeave(ctx, leaveReq, leaveRes)
		if err != nil {
			return nil, err
		}
		affected = append(affected, roomID)
		if len(outputEvents) == 0 {
			continue
		}
		if err := r.Inputer.OutputProducer.ProduceRoomEvents(roomID, outputEvents); err != nil {
			return nil, err
		}
	}
	return affected, nil
}

// PerformAdminPurgeRoom removes all traces for the given room from the database.
func (r *Admin) PerformAdminPurgeRoom(
	ctx context.Context,
	roomID string,
) error {
	// Validate we actually got a room ID and nothing else
	if _, _, err := gomatrixserverlib.SplitID('!', roomID); err != nil {
		return err
	}

	// Evacuate the room before purging it from the database
	evacAffected, err := r.PerformAdminEvacuateRoom(ctx, roomID)
	if err != nil {
		logrus.WithField("room_id", roomID).WithError(err).Warn("Failed to evacuate room before purging")
		return err
	}

	logrus.WithFields(logrus.Fields{
		"room_id":         roomID,
		"evacuated_users": len(evacAffected),
	}).Warn("Evacuated room, purging room from roomserver now")

	logrus.WithField("room_id", roomID).Warn("Purging room from roomserver")
	if err := r.DB.PurgeRoom(ctx, roomID); err != nil {
		logrus.WithField("room_id", roomID).WithError(err).Warn("Failed to purge room from roomserver")
		return err
	}

	logrus.WithField("room_id", roomID).Warn("Room purged from roomserver, informing other components")

	return r.Inputer.OutputProducer.ProduceRoomEvents(roomID, []api.OutputEvent{
		{
			Type: api.OutputTypePurgeRoom,
			PurgeRoom: &api.OutputPurgeRoom{
				RoomID: roomID,
			},
		},
	})
}

func (r *Admin) PerformAdminDownloadState(
	ctx context.Context,
	roomID, userID string, serverName spec.ServerName,
) error {
	fullUserID, err := spec.NewUserID(userID, true)
	if err != nil || fullUserID == nil {
		return err
	}
	senderDomain := fullUserID.Domain()

	roomInfo, err := r.DB.RoomInfo(ctx, roomID)
	if err != nil {
		return err
	}

	if roomInfo == nil || roomInfo.IsStub() {
		return eventutil.ErrRoomNoExists{}
	}

	fwdExtremities, _, depth, err := r.DB.LatestEventIDs(ctx, roomInfo.RoomNID)
	if err != nil {
		return err
	}

	authEventMap := map[string]gomatrixserverlib.PDU{}
	stateEventMap := map[string]gomatrixserverlib.PDU{}

	for _, fwdExtremity := range fwdExtremities {
		var state gomatrixserverlib.StateResponse
		state, err = r.Inputer.FSAPI.LookupState(ctx, r.Inputer.ServerName, serverName, roomID, fwdExtremity, roomInfo.RoomVersion)
		if err != nil {
			return fmt.Errorf("r.Inputer.FSAPI.LookupState (%q): %s", fwdExtremity, err)
		}
		for _, authEvent := range state.GetAuthEvents().UntrustedEvents(roomInfo.RoomVersion) {
			if err = gomatrixserverlib.VerifyEventSignatures(ctx, authEvent, r.Inputer.KeyRing, func(roomID string, senderID spec.SenderID) (*spec.UserID, error) {
				return r.DB.GetUserIDForSender(ctx, roomID, senderID)
			}); err != nil {
				continue
			}
			authEventMap[authEvent.EventID()] = authEvent
		}
		for _, stateEvent := range state.GetStateEvents().UntrustedEvents(roomInfo.RoomVersion) {
			if err = gomatrixserverlib.VerifyEventSignatures(ctx, stateEvent, r.Inputer.KeyRing, func(roomID string, senderID spec.SenderID) (*spec.UserID, error) {
				return r.DB.GetUserIDForSender(ctx, roomID, senderID)
			}); err != nil {
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

	senderID, err := r.Queryer.QuerySenderIDForUser(ctx, roomID, *fullUserID)
	if err != nil {
		return err
	}
	proto := &gomatrixserverlib.ProtoEvent{
		Type:     "org.matrix.dendrite.state_download",
		SenderID: string(senderID),
		RoomID:   roomID,
		Content:  spec.RawJSON("{}"),
	}

	eventsNeeded, err := gomatrixserverlib.StateNeededForProtoEvent(proto)
	if err != nil {
		return fmt.Errorf("gomatrixserverlib.StateNeededForProtoEvent: %w", err)
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

	ev, err := eventutil.BuildEvent(ctx, proto, identity, time.Now(), &eventsNeeded, queryRes)
	if err != nil {
		return fmt.Errorf("eventutil.BuildEvent: %w", err)
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

	r.Inputer.InputRoomEvents(ctx, inputReq, inputRes)

	if inputRes.ErrMsg != "" {
		return inputRes.Err()
	}

	return nil
}
