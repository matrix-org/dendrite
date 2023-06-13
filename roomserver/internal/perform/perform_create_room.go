// Copyright 2023 The Matrix.org Foundation C.I.C.
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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

const (
	historyVisibilityShared = "shared"
)

type Creator struct {
	DB    storage.Database
	Cfg   *config.RoomServer
	RSAPI api.RoomserverInternalAPI
}

// PerformCreateRoom handles all the steps necessary to create a new room.
// nolint: gocyclo
func (c *Creator) PerformCreateRoom(ctx context.Context, userID spec.UserID, roomID spec.RoomID, createRequest *api.PerformCreateRoomRequest) (string, *util.JSONResponse) {
	verImpl, err := gomatrixserverlib.GetRoomVersion(createRequest.RoomVersion)
	if err != nil {
		return "", &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("unknown room version"),
		}
	}

	createContent := map[string]interface{}{}
	if len(createRequest.CreationContent) > 0 {
		if err = json.Unmarshal(createRequest.CreationContent, &createContent); err != nil {
			util.GetLogger(ctx).WithError(err).Error("json.Unmarshal for creation_content failed")
			return "", &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("invalid create content"),
			}
		}
	}
	senderID, err := c.DB.GetSenderIDForUser(ctx, roomID.String(), userID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("Failed getting senderID for user")
		return "", &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	createContent["creator"] = senderID
	createContent["room_version"] = createRequest.RoomVersion
	powerLevelContent := eventutil.InitialPowerLevelsContent(string(senderID))
	joinRuleContent := gomatrixserverlib.JoinRuleContent{
		JoinRule: spec.Invite,
	}
	historyVisibilityContent := gomatrixserverlib.HistoryVisibilityContent{
		HistoryVisibility: historyVisibilityShared,
	}

	if createRequest.PowerLevelContentOverride != nil {
		// Merge powerLevelContentOverride fields by unmarshalling it atop the defaults
		err = json.Unmarshal(createRequest.PowerLevelContentOverride, &powerLevelContent)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("json.Unmarshal for power_level_content_override failed")
			return "", &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("malformed power_level_content_override"),
			}
		}
	}

	var guestsCanJoin bool
	switch createRequest.StatePreset {
	case spec.PresetPrivateChat:
		joinRuleContent.JoinRule = spec.Invite
		historyVisibilityContent.HistoryVisibility = historyVisibilityShared
		guestsCanJoin = true
	case spec.PresetTrustedPrivateChat:
		joinRuleContent.JoinRule = spec.Invite
		historyVisibilityContent.HistoryVisibility = historyVisibilityShared
		for _, invitee := range createRequest.InvitedUsers {
			powerLevelContent.Users[invitee] = 100
		}
		guestsCanJoin = true
	case spec.PresetPublicChat:
		joinRuleContent.JoinRule = spec.Public
		historyVisibilityContent.HistoryVisibility = historyVisibilityShared
	}

	createEvent := gomatrixserverlib.FledglingEvent{
		Type:    spec.MRoomCreate,
		Content: createContent,
	}
	powerLevelEvent := gomatrixserverlib.FledglingEvent{
		Type:    spec.MRoomPowerLevels,
		Content: powerLevelContent,
	}
	joinRuleEvent := gomatrixserverlib.FledglingEvent{
		Type:    spec.MRoomJoinRules,
		Content: joinRuleContent,
	}
	historyVisibilityEvent := gomatrixserverlib.FledglingEvent{
		Type:    spec.MRoomHistoryVisibility,
		Content: historyVisibilityContent,
	}
	membershipEvent := gomatrixserverlib.FledglingEvent{
		Type:     spec.MRoomMember,
		StateKey: string(senderID),
		Content: gomatrixserverlib.MemberContent{
			Membership:  spec.Join,
			DisplayName: createRequest.UserDisplayName,
			AvatarURL:   createRequest.UserAvatarURL,
		},
	}

	var nameEvent *gomatrixserverlib.FledglingEvent
	var topicEvent *gomatrixserverlib.FledglingEvent
	var guestAccessEvent *gomatrixserverlib.FledglingEvent
	var aliasEvent *gomatrixserverlib.FledglingEvent

	if createRequest.RoomName != "" {
		nameEvent = &gomatrixserverlib.FledglingEvent{
			Type: spec.MRoomName,
			Content: eventutil.NameContent{
				Name: createRequest.RoomName,
			},
		}
	}

	if createRequest.Topic != "" {
		topicEvent = &gomatrixserverlib.FledglingEvent{
			Type: spec.MRoomTopic,
			Content: eventutil.TopicContent{
				Topic: createRequest.Topic,
			},
		}
	}

	if guestsCanJoin {
		guestAccessEvent = &gomatrixserverlib.FledglingEvent{
			Type: spec.MRoomGuestAccess,
			Content: eventutil.GuestAccessContent{
				GuestAccess: "can_join",
			},
		}
	}

	var roomAlias string
	if createRequest.RoomAliasName != "" {
		roomAlias = fmt.Sprintf("#%s:%s", createRequest.RoomAliasName, userID.Domain())
		// check it's free
		// TODO: This races but is better than nothing
		hasAliasReq := api.GetRoomIDForAliasRequest{
			Alias:              roomAlias,
			IncludeAppservices: false,
		}

		var aliasResp api.GetRoomIDForAliasResponse
		err = c.RSAPI.GetRoomIDForAlias(ctx, &hasAliasReq, &aliasResp)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("aliasAPI.GetRoomIDForAlias failed")
			return "", &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		if aliasResp.RoomID != "" {
			return "", &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.RoomInUse("Room ID already exists."),
			}
		}

		aliasEvent = &gomatrixserverlib.FledglingEvent{
			Type: spec.MRoomCanonicalAlias,
			Content: eventutil.CanonicalAlias{
				Alias: roomAlias,
			},
		}
	}

	var initialStateEvents []gomatrixserverlib.FledglingEvent
	for i := range createRequest.InitialState {
		if createRequest.InitialState[i].StateKey != "" {
			initialStateEvents = append(initialStateEvents, createRequest.InitialState[i])
			continue
		}

		switch createRequest.InitialState[i].Type {
		case spec.MRoomCreate:
			continue

		case spec.MRoomPowerLevels:
			powerLevelEvent = createRequest.InitialState[i]

		case spec.MRoomJoinRules:
			joinRuleEvent = createRequest.InitialState[i]

		case spec.MRoomHistoryVisibility:
			historyVisibilityEvent = createRequest.InitialState[i]

		case spec.MRoomGuestAccess:
			guestAccessEvent = &createRequest.InitialState[i]

		case spec.MRoomName:
			nameEvent = &createRequest.InitialState[i]

		case spec.MRoomTopic:
			topicEvent = &createRequest.InitialState[i]

		default:
			initialStateEvents = append(initialStateEvents, createRequest.InitialState[i])
		}
	}

	// send events into the room in order of:
	//  1- m.room.create
	//  2- room creator join member
	//  3- m.room.power_levels
	//  4- m.room.join_rules
	//  5- m.room.history_visibility
	//  6- m.room.canonical_alias (opt)
	//  7- m.room.guest_access (opt)
	//  8- other initial state items
	//  9- m.room.name (opt)
	//  10- m.room.topic (opt)
	//  11- invite events (opt) - with is_direct flag if applicable TODO
	//  12- 3pid invite events (opt) TODO
	// This differs from Synapse slightly. Synapse would vary the ordering of 3-7
	// depending on if those events were in "initial_state" or not. This made it
	// harder to reason about, hence sticking to a strict static ordering.
	// TODO: Synapse has txn/token ID on each event. Do we need to do this here?
	eventsToMake := []gomatrixserverlib.FledglingEvent{
		createEvent, membershipEvent, powerLevelEvent, joinRuleEvent, historyVisibilityEvent,
	}
	if guestAccessEvent != nil {
		eventsToMake = append(eventsToMake, *guestAccessEvent)
	}
	eventsToMake = append(eventsToMake, initialStateEvents...)
	if nameEvent != nil {
		eventsToMake = append(eventsToMake, *nameEvent)
	}
	if topicEvent != nil {
		eventsToMake = append(eventsToMake, *topicEvent)
	}
	if aliasEvent != nil {
		// TODO: bit of a chicken and egg problem here as the alias doesn't exist and cannot until we have made the room.
		// This means we might fail creating the alias but say the canonical alias is something that doesn't exist.
		eventsToMake = append(eventsToMake, *aliasEvent)
	}

	// TODO: invite events
	// TODO: 3pid invite events

	var builtEvents []*types.HeaderedEvent
	authEvents := gomatrixserverlib.NewAuthEvents(nil)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("rsapi.QuerySenderIDForUser failed")
		return "", &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	for i, e := range eventsToMake {
		depth := i + 1 // depth starts at 1

		builder := verImpl.NewEventBuilderFromProtoEvent(&gomatrixserverlib.ProtoEvent{
			SenderID: string(senderID),
			RoomID:   roomID.String(),
			Type:     e.Type,
			StateKey: &e.StateKey,
			Depth:    int64(depth),
		})
		err = builder.SetContent(e.Content)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("builder.SetContent failed")
			return "", &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		if i > 0 {
			builder.PrevEvents = []string{builtEvents[i-1].EventID()}
		}
		var ev gomatrixserverlib.PDU
		if err = builder.AddAuthEvents(&authEvents); err != nil {
			util.GetLogger(ctx).WithError(err).Error("AddAuthEvents failed")
			return "", &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		ev, err = builder.Build(createRequest.EventTime, userID.Domain(), createRequest.KeyID, createRequest.PrivateKey)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("buildEvent failed")
			return "", &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		if err = gomatrixserverlib.Allowed(ev, &authEvents, func(roomID string, senderID spec.SenderID) (*spec.UserID, error) {
			return c.DB.GetUserIDForSender(ctx, roomID, senderID)
		}); err != nil {
			util.GetLogger(ctx).WithError(err).Error("gomatrixserverlib.Allowed failed")
			return "", &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		// Add the event to the list of auth events
		builtEvents = append(builtEvents, &types.HeaderedEvent{PDU: ev})
		err = authEvents.AddEvent(ev)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("authEvents.AddEvent failed")
			return "", &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
	}

	inputs := make([]api.InputRoomEvent, 0, len(builtEvents))
	for _, event := range builtEvents {
		inputs = append(inputs, api.InputRoomEvent{
			Kind:         api.KindNew,
			Event:        event,
			Origin:       userID.Domain(),
			SendAsServer: api.DoNotSendToOtherServers,
		})
	}

	// first send the `m.room.create` event, so we have a roomNID
	if err = api.SendInputRoomEvents(ctx, c.RSAPI, userID.Domain(), inputs[:1], false); err != nil {
		util.GetLogger(ctx).WithError(err).Error("roomserverAPI.SendInputRoomEvents failed")
		return "", &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// create user room key if needed
	if createRequest.RoomVersion == gomatrixserverlib.RoomVersionPseudoIDs {
		_, err = c.RSAPI.GetOrCreateUserRoomPrivateKey(ctx, userID, roomID)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("GetOrCreateUserRoomPrivateKey failed")
			return "", &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
	}

	// send the remaining events
	if err = api.SendInputRoomEvents(ctx, c.RSAPI, userID.Domain(), inputs[1:], false); err != nil {
		util.GetLogger(ctx).WithError(err).Error("roomserverAPI.SendInputRoomEvents failed")
		return "", &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// TODO(#269): Reserve room alias while we create the room. This stops us
	// from creating the room but still failing due to the alias having already
	// been taken.
	if roomAlias != "" {
		aliasReq := api.SetRoomAliasRequest{
			Alias:  roomAlias,
			RoomID: roomID.String(),
			UserID: userID.String(),
		}

		var aliasResp api.SetRoomAliasResponse
		err = c.RSAPI.SetRoomAlias(ctx, &aliasReq, &aliasResp)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("aliasAPI.SetRoomAlias failed")
			return "", &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		if aliasResp.AliasExists {
			return "", &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.RoomInUse("Room alias already exists."),
			}
		}
	}

	// If this is a direct message then we should invite the participants.
	if len(createRequest.InvitedUsers) > 0 {
		// Build some stripped state for the invite.
		var globalStrippedState []gomatrixserverlib.InviteStrippedState
		for _, event := range builtEvents {
			// Chosen events from the spec:
			// https://spec.matrix.org/v1.3/client-server-api/#stripped-state
			switch event.Type() {
			case spec.MRoomCreate:
				fallthrough
			case spec.MRoomName:
				fallthrough
			case spec.MRoomAvatar:
				fallthrough
			case spec.MRoomTopic:
				fallthrough
			case spec.MRoomCanonicalAlias:
				fallthrough
			case spec.MRoomEncryption:
				fallthrough
			case spec.MRoomMember:
				fallthrough
			case spec.MRoomJoinRules:
				ev := event.PDU
				globalStrippedState = append(
					globalStrippedState,
					gomatrixserverlib.NewInviteStrippedState(ev),
				)
			}
		}

		// Process the invites.
		var inviteEvent *types.HeaderedEvent
		for _, invitee := range createRequest.InvitedUsers {
			inviteeUserID, userIDErr := spec.NewUserID(invitee, true)
			if userIDErr != nil {
				util.GetLogger(ctx).WithError(userIDErr).Error("invalid UserID")
				return "", &util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}
			inviteeSenderID, queryErr := c.RSAPI.QuerySenderIDForUser(ctx, roomID.String(), *inviteeUserID)
			if queryErr != nil {
				util.GetLogger(ctx).WithError(queryErr).Error("rsapi.QuerySenderIDForUser failed")
				return "", &util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}
			inviteeString := string(inviteeSenderID)
			proto := gomatrixserverlib.ProtoEvent{
				SenderID: string(senderID),
				RoomID:   roomID.String(),
				Type:     "m.room.member",
				StateKey: &inviteeString,
			}

			content := gomatrixserverlib.MemberContent{
				Membership:  spec.Invite,
				DisplayName: createRequest.UserDisplayName,
				AvatarURL:   createRequest.UserAvatarURL,
				Reason:      "",
				IsDirect:    createRequest.IsDirect,
			}

			if err = proto.SetContent(content); err != nil {
				return "", &util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}

			// Build the invite event.
			identity := &fclient.SigningIdentity{
				ServerName: userID.Domain(),
				KeyID:      createRequest.KeyID,
				PrivateKey: createRequest.PrivateKey,
			}
			inviteEvent, err = eventutil.QueryAndBuildEvent(ctx, &proto, identity, createRequest.EventTime, c.RSAPI, nil)

			if err != nil {
				util.GetLogger(ctx).WithError(err).Error("buildMembershipEvent failed")
				continue
			}
			inviteStrippedState := append(
				globalStrippedState,
				gomatrixserverlib.NewInviteStrippedState(inviteEvent.PDU),
			)
			// Send the invite event to the roomserver.
			event := inviteEvent
			err = c.RSAPI.PerformInvite(ctx, &api.PerformInviteRequest{
				Event:           event,
				InviteRoomState: inviteStrippedState,
				RoomVersion:     event.Version(),
				SendAsServer:    string(userID.Domain()),
			})
			switch e := err.(type) {
			case api.ErrInvalidID:
				return "", &util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.Unknown(e.Error()),
				}
			case api.ErrNotAllowed:
				return "", &util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden(e.Error()),
				}
			case nil:
			default:
				util.GetLogger(ctx).WithError(err).Error("PerformInvite failed")
				sentry.CaptureException(err)
				return "", &util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}
		}
	}

	if createRequest.Visibility == spec.Public {
		// expose this room in the published room list
		if err = c.RSAPI.PerformPublish(ctx, &api.PerformPublishRequest{
			RoomID:     roomID.String(),
			Visibility: spec.Public,
		}); err != nil {
			util.GetLogger(ctx).WithError(err).Error("failed to publish room")
			return "", &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
	}

	// TODO: visibility/presets/raw initial state
	// TODO: Create room alias association
	// Make sure this doesn't fall into an application service's namespace though!

	return roomAlias, nil
}
