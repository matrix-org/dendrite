// Copyright 2022 New Vector Ltd
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
	"time"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type Upgrader struct {
	Cfg    *config.RoomServer
	URSAPI api.RoomserverInternalAPI
}

// fledglingEvent is a helper representation of an event used when creating many events in succession.
type fledglingEvent struct {
	Type     string      `json:"type"`
	StateKey string      `json:"state_key"`
	Content  interface{} `json:"content"`
}

// PerformRoomUpgrade upgrades a room from one version to another
func (r *Upgrader) PerformRoomUpgrade(
	ctx context.Context,
	req *api.PerformRoomUpgradeRequest,
	res *api.PerformRoomUpgradeResponse,
) {
	res.NewRoomID, res.Error = r.performRoomUpgrade(ctx, req)
	if res.Error != nil {
		res.NewRoomID = ""
		logrus.WithContext(ctx).WithError(res.Error).Error("Room upgrade failed")
	}
}

func (r *Upgrader) performRoomUpgrade(
	ctx context.Context,
	req *api.PerformRoomUpgradeRequest,
) (string, *api.PerformError) {
	roomID := req.RoomID
	userID := req.UserID
	evTime := time.Now()

	// Return an immediate error if the room does not exist
	if err := r.validateRoomExists(ctx, roomID); err != nil {
		return "", &api.PerformError{
			Code: api.PerformErrorNoRoom,
			Msg:  "Error validating that the room exists",
		}
	}

	// 1. Check if the user is authorized to actually perform the upgrade (can send m.room.tombstone)
	if !r.userIsAuthorized(ctx, userID, roomID) {
		return "", &api.PerformError{
			Code: api.PerformErrorNotAllowed,
			Msg:  "You don't have permission to upgrade the room, power level too low.",
		}

	}

	// TODO (#267): Check room ID doesn't clash with an existing one, and we
	//              probably shouldn't be using pseudo-random strings, maybe GUIDs?
	newRoomID := fmt.Sprintf("!%s:%s", util.RandomString(16), r.Cfg.Matrix.ServerName)

	// Make the tombstone event
	tombstoneEvent, pErr := r.makeTombstoneEvent(ctx, evTime, userID, roomID, newRoomID)
	if pErr != nil {
		return "", pErr
	}

	// Generate the initial events we need to send into the new room. This includes copied state events and bans
	// as well as the power level events needed to set up the room
	eventsToMake, pErr := r.generateInitialEvents(ctx, userID, roomID, string(req.RoomVersion), tombstoneEvent)
	if pErr != nil {
		return "", pErr
	}

	// 5. Send the tombstone event to the old room (must do this before we set the new canonical_alias)
	if pErr = r.sendHeaderedEvent(ctx, tombstoneEvent); pErr != nil {
		return "", pErr
	}

	// Send the setup events to the new room
	if pErr = r.sendInitialEvents(ctx, evTime, userID, newRoomID, string(req.RoomVersion), eventsToMake); pErr != nil {
		return "", pErr
	}

	// If the old room was public, make sure the new one is too
	if pErr = r.publishIfOldRoomWasPublic(ctx, roomID, newRoomID); pErr != nil {
		return "", pErr
	}

	// If the old room had a canonical alias event, it should be deleted in the old room
	if pErr = r.clearOldCanonicalAliasEvent(ctx, evTime, userID, roomID); pErr != nil {
		return "", pErr
	}

	// 4. Move local aliases to the new room
	if pErr = moveLocalAliases(ctx, roomID, newRoomID, userID, r.URSAPI); pErr != nil {
		return "", pErr
	}

	// 6. Restrict power levels in the old room
	if pErr = r.restrictOldRoomPowerLevels(ctx, evTime, userID, roomID); pErr != nil {
		return "", pErr
	}

	return newRoomID, nil
}

func (r *Upgrader) getRoomPowerLevels(ctx context.Context, roomID string) (*gomatrixserverlib.PowerLevelContent, *api.PerformError) {
	oldPowerLevelsEvent := api.GetStateEvent(ctx, r.URSAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomPowerLevels,
		StateKey:  "",
	})
	powerLevelContent, err := oldPowerLevelsEvent.PowerLevels()
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error()
		return nil, &api.PerformError{
			Msg: "powerLevel event was not actually a power level event",
		}
	}
	return powerLevelContent, nil
}

func (r *Upgrader) restrictOldRoomPowerLevels(ctx context.Context, evTime time.Time, userID, roomID string) *api.PerformError {
	powerLevelContent, pErr := r.getRoomPowerLevels(ctx, roomID)
	if pErr != nil {
		return pErr
	}

	restrictedPowerLevelContent := &gomatrixserverlib.PowerLevelContent{}
	*restrictedPowerLevelContent = *powerLevelContent

	restrictedDefaultPowerLevel := int64(50)
	if restrictedPowerLevelContent.UsersDefault+1 > restrictedDefaultPowerLevel {
		restrictedDefaultPowerLevel = restrictedPowerLevelContent.UsersDefault + 1
	}
	restrictedPowerLevelContent.EventsDefault = restrictedDefaultPowerLevel
	restrictedPowerLevelContent.Invite = restrictedDefaultPowerLevel

	restrictedPowerLevelsHeadered, resErr := r.makeHeaderedEvent(ctx, evTime, userID, roomID, fledglingEvent{
		Type:    gomatrixserverlib.MRoomPowerLevels,
		Content: restrictedPowerLevelContent,
	})
	if resErr != nil {
		if resErr.Code == api.PerformErrorNotAllowed {
			util.GetLogger(ctx).WithField(logrus.ErrorKey, resErr).Warn("UpgradeRoom: Could not restrict power levels in old room")
		} else {
			return resErr
		}
	} else {
		if resErr = r.sendHeaderedEvent(ctx, restrictedPowerLevelsHeadered); resErr != nil {
			return resErr
		}
	}
	return nil
}

func moveLocalAliases(ctx context.Context,
	roomID, newRoomID, userID string,
	URSAPI api.RoomserverInternalAPI) *api.PerformError {
	var err error

	aliasReq := api.GetAliasesForRoomIDRequest{RoomID: roomID}
	aliasRes := api.GetAliasesForRoomIDResponse{}
	if err = URSAPI.GetAliasesForRoomID(ctx, &aliasReq, &aliasRes); err != nil {
		return &api.PerformError{
			Msg: "Could not get aliases for old room",
		}
	}

	for _, alias := range aliasRes.Aliases {
		removeAliasReq := api.RemoveRoomAliasRequest{UserID: userID, Alias: alias}
		removeAliasRes := api.RemoveRoomAliasResponse{}
		if err = URSAPI.RemoveRoomAlias(ctx, &removeAliasReq, &removeAliasRes); err != nil {
			return &api.PerformError{
				Msg: "api.RemoveRoomAlias failed",
			}
		}

		setAliasReq := api.SetRoomAliasRequest{UserID: userID, Alias: alias, RoomID: newRoomID}
		setAliasRes := api.SetRoomAliasResponse{}
		if err = URSAPI.SetRoomAlias(ctx, &setAliasReq, &setAliasRes); err != nil {
			return &api.PerformError{
				Msg: "api.SetRoomAlias failed",
			}
		}
	}
	return nil
}

func (r *Upgrader) clearOldCanonicalAliasEvent(ctx context.Context, evTime time.Time, userID, roomID string) *api.PerformError {
	emptyCanonicalAliasEvent, resErr := r.makeHeaderedEvent(ctx, evTime, userID, roomID, fledglingEvent{
		Type:    gomatrixserverlib.MRoomCanonicalAlias,
		Content: map[string]interface{}{},
	})
	if resErr != nil {
		if resErr.Code == api.PerformErrorNotAllowed {
			util.GetLogger(ctx).WithField(logrus.ErrorKey, resErr).Warn("UpgradeRoom: Could not set empty canonical alias event in old room")
		} else {
			return resErr
		}
	} else {
		if resErr = r.sendHeaderedEvent(ctx, emptyCanonicalAliasEvent); resErr != nil {
			return resErr
		}
	}
	return nil
}

func (r *Upgrader) publishIfOldRoomWasPublic(ctx context.Context, roomID, newRoomID string) *api.PerformError {
	// check if the old room was published
	var pubQueryRes api.QueryPublishedRoomsResponse
	err := r.URSAPI.QueryPublishedRooms(ctx, &api.QueryPublishedRoomsRequest{
		RoomID: roomID,
	}, &pubQueryRes)
	if err != nil {
		return &api.PerformError{
			Msg: "QueryPublishedRooms failed",
		}
	}

	// if the old room is published (was public), publish the new room
	if len(pubQueryRes.RoomIDs) == 1 {
		publishNewRoom(ctx, r.URSAPI, roomID, newRoomID)
	}
	return nil
}

func publishNewRoom(
	ctx context.Context,
	URSAPI api.RoomserverInternalAPI,
	oldRoomID, newRoomID string,
) {
	// expose this room in the published room list
	var pubNewRoomRes api.PerformPublishResponse
	URSAPI.PerformPublish(ctx, &api.PerformPublishRequest{
		RoomID:     newRoomID,
		Visibility: "public",
	}, &pubNewRoomRes)
	if pubNewRoomRes.Error != nil {
		// treat as non-fatal since the room is already made by this point
		util.GetLogger(ctx).WithError(pubNewRoomRes.Error).Error("failed to visibility:public")
	}

	var unpubOldRoomRes api.PerformPublishResponse
	// remove the old room from the published room list
	URSAPI.PerformPublish(ctx, &api.PerformPublishRequest{
		RoomID:     oldRoomID,
		Visibility: "private",
	}, &unpubOldRoomRes)
	if unpubOldRoomRes.Error != nil {
		// treat as non-fatal since the room is already made by this point
		util.GetLogger(ctx).WithError(unpubOldRoomRes.Error).Error("failed to visibility:private")
	}
}

func (r *Upgrader) validateRoomExists(ctx context.Context, roomID string) error {
	verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
	verRes := api.QueryRoomVersionForRoomResponse{}
	if err := r.URSAPI.QueryRoomVersionForRoom(ctx, &verReq, &verRes); err != nil {
		return &api.PerformError{
			Code: api.PerformErrorNoRoom,
			Msg:  "Room does not exist",
		}
	}
	return nil
}

func (r *Upgrader) userIsAuthorized(ctx context.Context, userID, roomID string,
) bool {
	plEvent := api.GetStateEvent(ctx, r.URSAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomPowerLevels,
		StateKey:  "",
	})
	if plEvent == nil {
		return false
	}
	pl, err := plEvent.PowerLevels()
	if err != nil {
		return false
	}
	// Check for power level required to send tombstone event (marks the curren room as obsolete),
	// if not found, use the StateDefault power level
	plToUpgrade, ok := pl.Events["m.room.tombstone"]
	if !ok {
		plToUpgrade = pl.StateDefault
	}
	return pl.UserLevel(userID) >= plToUpgrade
}

// nolint:composites,gocyclo
func (r *Upgrader) generateInitialEvents(ctx context.Context, userID, roomID, newVersion string, tombstoneEvent *gomatrixserverlib.HeaderedEvent) ([]fledglingEvent, *api.PerformError) {
	req := &api.QueryLatestEventsAndStateRequest{
		RoomID: roomID,
	}
	res := &api.QueryLatestEventsAndStateResponse{}
	if err := r.URSAPI.QueryLatestEventsAndState(ctx, req, res); err != nil {
		return nil, &api.PerformError{
			Msg: fmt.Sprintf("Failed to get latest state: %s", err),
		}
	}
	state := make(map[gomatrixserverlib.StateKeyTuple]*gomatrixserverlib.HeaderedEvent, len(res.StateEvents))
	for _, event := range res.StateEvents {
		if event.StateKey() == nil {
			continue // shouldn't ever happen, but better to be safe than sorry
		}
		tuple := gomatrixserverlib.StateKeyTuple{EventType: event.Type(), StateKey: *event.StateKey()}
		state[tuple] = event
	}

	oldCreateEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomCreate, StateKey: "",
	}]
	oldMembershipEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomMember, StateKey: userID,
	}]
	oldPowerLevelsEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomPowerLevels, StateKey: "",
	}]
	oldJoinRulesEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomJoinRules, StateKey: "",
	}]
	oldHistoryVisibilityEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomHistoryVisibility, StateKey: "",
	}]
	oldNameEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomName, StateKey: "",
	}]
	oldTopicEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomTopic, StateKey: "",
	}]
	oldGuestAccessEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomGuestAccess, StateKey: "",
	}]
	oldAvatarEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomAvatar, StateKey: "",
	}]
	oldEncryptionEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomEncryption, StateKey: "",
	}]
	oldCanonicalAliasEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomCanonicalAlias, StateKey: "",
	}]
	oldServerAclEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: "m.room.server_acl", StateKey: "",
	}]
	oldRelatedGroupsEvent := state[gomatrixserverlib.StateKeyTuple{
		EventType: "m.room.related_groups", StateKey: "",
	}]

	newCreateContent := map[string]interface{}{
		"creator":      userID,
		"room_version": newVersion, // TODO: change struct to single var?
		"predecessor": gomatrixserverlib.PreviousRoom{
			EventID: tombstoneEvent.EventID(),
			RoomID:  roomID,
		},
	}
	oldCreateContent := unmarshal(oldCreateEvent.Content())
	if federate, ok := oldCreateContent["m.federate"].(bool); ok {
		newCreateContent["m.federate"] = federate
	}

	newCreateEvent := fledglingEvent{
		Type:    gomatrixserverlib.MRoomCreate,
		Content: newCreateContent,
	}

	membershipContent := gomatrixserverlib.MemberContent{
		Membership: gomatrixserverlib.Join,
	}
	if err := json.Unmarshal(oldMembershipEvent.Content(), &membershipContent); err != nil {
		util.GetLogger(ctx).WithError(err).Error()
		return nil, &api.PerformError{
			Msg: "Membership event content was invalid",
		}
	}
	membershipContent.Membership = gomatrixserverlib.Join
	membershipEvent := fledglingEvent{
		Type:     gomatrixserverlib.MRoomMember,
		StateKey: userID,
		Content:  membershipContent,
	}

	powerLevelContent, err := oldPowerLevelsEvent.PowerLevels()
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error()
		return nil, &api.PerformError{
			Msg: "powerLevel event was not actually a power level event",
		}
	}

	newPowerLevelsEvent := fledglingEvent{
		Type:    gomatrixserverlib.MRoomPowerLevels,
		Content: powerLevelContent,
	}

	//create temporary power level event that elevates upgrading user's prvileges to create every copied state event
	tempPowerLevelsEvent := createTemporaryPowerLevels(powerLevelContent, userID)

	joinRulesContent, err := oldJoinRulesEvent.JoinRule()
	if err != nil {
		return nil, &api.PerformError{
			Msg: "Join rules event had bad content",
		}
	}
	newJoinRulesEvent := fledglingEvent{
		Type: gomatrixserverlib.MRoomJoinRules,
		Content: map[string]interface{}{
			"join_rule": joinRulesContent,
		},
	}

	historyVisibilityContent, err := oldHistoryVisibilityEvent.HistoryVisibility()
	if err != nil {
		return nil, &api.PerformError{
			Msg: "History visibility event had bad content",
		}
	}
	newHistoryVisibilityEvent := fledglingEvent{
		Type: gomatrixserverlib.MRoomHistoryVisibility,
		Content: map[string]interface{}{
			"history_visibility": historyVisibilityContent,
		},
	}

	var newNameEvent fledglingEvent
	var newTopicEvent fledglingEvent
	var newGuestAccessEvent fledglingEvent
	var newAvatarEvent fledglingEvent
	var newEncryptionEvent fledglingEvent
	var newServerACLEvent fledglingEvent
	var newRelatedGroupsEvent fledglingEvent
	var newCanonicalAliasEvent fledglingEvent

	if oldNameEvent != nil {
		newNameEvent = fledglingEvent{
			Type:    gomatrixserverlib.MRoomName,
			Content: unmarshal(oldNameEvent.Content()),
		}
	}
	if oldTopicEvent != nil {
		newTopicEvent = fledglingEvent{
			Type:    gomatrixserverlib.MRoomTopic,
			Content: unmarshal(oldTopicEvent.Content()),
		}
	}
	if oldGuestAccessEvent != nil {
		newGuestAccessEvent = fledglingEvent{
			Type:    gomatrixserverlib.MRoomGuestAccess,
			Content: unmarshal(oldGuestAccessEvent.Content()),
		}
	}
	if oldAvatarEvent != nil {
		newAvatarEvent = fledglingEvent{
			Type:    gomatrixserverlib.MRoomAvatar,
			Content: unmarshal(oldAvatarEvent.Content()),
		}
	}
	if oldEncryptionEvent != nil {
		newEncryptionEvent = fledglingEvent{
			Type:    gomatrixserverlib.MRoomEncryption,
			Content: unmarshal(oldEncryptionEvent.Content()),
		}
	}
	if oldServerAclEvent != nil {
		newServerACLEvent = fledglingEvent{
			Type:    "m.room.server_acl",
			Content: unmarshal(oldServerAclEvent.Content()),
		}
	}
	if oldRelatedGroupsEvent != nil {
		newRelatedGroupsEvent = fledglingEvent{
			Type:    "m.room.related_groups",
			Content: unmarshal(oldRelatedGroupsEvent.Content()),
		}
	}
	if oldCanonicalAliasEvent != nil {
		newCanonicalAliasEvent = fledglingEvent{
			Type:    gomatrixserverlib.MRoomCanonicalAlias,
			Content: unmarshal(oldCanonicalAliasEvent.Content()),
		}
	}

	// 3. Replicate transferable state events
	//  send events into the room in order of:
	//  1- m.room.create
	//  2- m.room.power_levels (temporary, to allow the upgrading user to send everything)
	//  3- m.room.join_rules
	//  4- m.room.history_visibility
	//  5- m.room.guest_access
	//  6- m.room.name
	//	7- m.room.avatar
	//  8- m.room.topic
	//	9- m.room.encryption
	//	10-m.room.server_acl
	//  11-m.room.related_groups
	//  12-m.room.canonical_alias
	//  13-All ban events from the old room
	//  14-The original room power levels
	eventsToMake := []fledglingEvent{
		newCreateEvent, membershipEvent, tempPowerLevelsEvent, newJoinRulesEvent, newHistoryVisibilityEvent,
	}
	if oldGuestAccessEvent != nil {
		eventsToMake = append(eventsToMake, newGuestAccessEvent)
	} else { // Always create this with the default value to appease sytests
		eventsToMake = append(eventsToMake, fledglingEvent{
			Type:    gomatrixserverlib.MRoomGuestAccess,
			Content: map[string]interface{}{"guest_access": "forbidden"},
		})
	}
	if oldNameEvent != nil {
		eventsToMake = append(eventsToMake, newNameEvent)
	}
	if oldAvatarEvent != nil {
		eventsToMake = append(eventsToMake, newAvatarEvent)
	}
	if oldTopicEvent != nil {
		eventsToMake = append(eventsToMake, newTopicEvent)
	}
	if oldEncryptionEvent != nil {
		eventsToMake = append(eventsToMake, newEncryptionEvent)
	}
	if oldServerAclEvent != nil {
		eventsToMake = append(eventsToMake, newServerACLEvent)
	}
	if oldRelatedGroupsEvent != nil {
		eventsToMake = append(eventsToMake, newRelatedGroupsEvent)
	}
	if oldCanonicalAliasEvent != nil {
		eventsToMake = append(eventsToMake, newCanonicalAliasEvent)
	}
	banEvents, err := getBanEvents(ctx, roomID, r.URSAPI)
	if err != nil {
		return nil, &api.PerformError{
			Msg: err.Error(),
		}
	} else {
		eventsToMake = append(eventsToMake, banEvents...)
	}
	eventsToMake = append(eventsToMake, newPowerLevelsEvent)
	return eventsToMake, nil
}

func (r *Upgrader) sendInitialEvents(ctx context.Context, evTime time.Time, userID, newRoomID, newVersion string, eventsToMake []fledglingEvent) *api.PerformError {
	var err error
	var builtEvents []*gomatrixserverlib.HeaderedEvent
	authEvents := gomatrixserverlib.NewAuthEvents(nil)
	for i, e := range eventsToMake {
		depth := i + 1 // depth starts at 1

		builder := gomatrixserverlib.EventBuilder{
			Sender:   userID,
			RoomID:   newRoomID,
			Type:     e.Type,
			StateKey: &e.StateKey,
			Depth:    int64(depth),
		}
		err = builder.SetContent(e.Content)
		if err != nil {
			return &api.PerformError{
				Msg: "builder.SetContent failed",
			}
		}
		if i > 0 {
			builder.PrevEvents = []gomatrixserverlib.EventReference{builtEvents[i-1].EventReference()}
		}
		var event *gomatrixserverlib.Event
		event, err = r.buildEvent(&builder, &authEvents, evTime, gomatrixserverlib.RoomVersion(newVersion))
		if err != nil {
			return &api.PerformError{
				Msg: "buildEvent failed",
			}
		}

		if err = gomatrixserverlib.Allowed(event, &authEvents); err != nil {
			return &api.PerformError{
				Msg: "gomatrixserverlib.Allowed failed",
			}
		}

		// Add the event to the list of auth events
		builtEvents = append(builtEvents, event.Headered(gomatrixserverlib.RoomVersion(newVersion)))
		err = authEvents.AddEvent(event)
		if err != nil {
			return &api.PerformError{
				Msg: "authEvents.AddEvent failed",
			}
		}
	}

	inputs := make([]api.InputRoomEvent, 0, len(builtEvents))
	for _, event := range builtEvents {
		inputs = append(inputs, api.InputRoomEvent{
			Kind:         api.KindNew,
			Event:        event,
			Origin:       r.Cfg.Matrix.ServerName,
			SendAsServer: api.DoNotSendToOtherServers,
		})
	}
	if err = api.SendInputRoomEvents(ctx, r.URSAPI, inputs, false); err != nil {
		return &api.PerformError{
			Msg: "api.SendInputRoomEvents failed",
		}
	}
	return nil
}

func (r *Upgrader) makeTombstoneEvent(
	ctx context.Context,
	evTime time.Time,
	userID, roomID, newRoomID string,
) (*gomatrixserverlib.HeaderedEvent, *api.PerformError) {
	content := map[string]interface{}{
		"body":             "This room has been replaced",
		"replacement_room": newRoomID,
	}
	event := fledglingEvent{
		Type:    "m.room.tombstone",
		Content: content,
	}
	return r.makeHeaderedEvent(ctx, evTime, userID, roomID, event)
}

func (r *Upgrader) makeHeaderedEvent(ctx context.Context, evTime time.Time, userID, roomID string, event fledglingEvent) (*gomatrixserverlib.HeaderedEvent, *api.PerformError) {
	builder := gomatrixserverlib.EventBuilder{
		Sender:   userID,
		RoomID:   roomID,
		Type:     event.Type,
		StateKey: &event.StateKey,
	}
	err := builder.SetContent(event.Content)
	if err != nil {
		return nil, &api.PerformError{
			Msg: "builder.SetContent failed",
		}
	}
	var queryRes api.QueryLatestEventsAndStateResponse
	headeredEvent, err := eventutil.QueryAndBuildEvent(ctx, &builder, r.Cfg.Matrix, evTime, r.URSAPI, &queryRes)
	if err == eventutil.ErrRoomNoExists {
		return nil, &api.PerformError{
			Code: api.PerformErrorNoRoom,
			Msg:  "Room does not exist",
		}
	} else if e, ok := err.(gomatrixserverlib.BadJSONError); ok {
		return nil, &api.PerformError{
			Msg: e.Error(),
		}
	} else if e, ok := err.(gomatrixserverlib.EventValidationError); ok {
		if e.Code == gomatrixserverlib.EventValidationTooLarge {
			return nil, &api.PerformError{
				Msg: e.Error(),
			}
		}
		return nil, &api.PerformError{
			Msg: e.Error(),
		}
	} else if err != nil {
		return nil, &api.PerformError{
			Msg: "eventutil.BuildEvent failed",
		}
	}
	// check to see if this user can perform this operation
	stateEvents := make([]*gomatrixserverlib.Event, len(queryRes.StateEvents))
	for i := range queryRes.StateEvents {
		stateEvents[i] = queryRes.StateEvents[i].Event
	}
	provider := gomatrixserverlib.NewAuthEvents(stateEvents)
	if err = gomatrixserverlib.Allowed(headeredEvent.Event, &provider); err != nil {
		return nil, &api.PerformError{
			Code: api.PerformErrorNotAllowed,
			Msg:  err.Error(), // TODO: Is this error string comprehensible to the client?
		}
	}

	return headeredEvent, nil
}

func getBanEvents(ctx context.Context, roomID string, URSAPI api.RoomserverInternalAPI) ([]fledglingEvent, error) {
	var err error
	banEvents := []fledglingEvent{}

	roomMemberReq := api.QueryCurrentStateRequest{RoomID: roomID, AllowWildcards: true, StateTuples: []gomatrixserverlib.StateKeyTuple{
		{EventType: gomatrixserverlib.MRoomMember, StateKey: "*"},
	}}
	roomMemberRes := api.QueryCurrentStateResponse{}
	if err = URSAPI.QueryCurrentState(ctx, &roomMemberReq, &roomMemberRes); err != nil {
		return nil, err
	}
	for _, event := range roomMemberRes.StateEvents {
		if event == nil {
			continue
		}
		memberContent, err := gomatrixserverlib.NewMemberContentFromEvent(event.Event)
		if err != nil || memberContent.Membership != gomatrixserverlib.Ban {
			continue
		}
		banEvents = append(banEvents, fledglingEvent{Type: gomatrixserverlib.MRoomMember, StateKey: *event.StateKey(), Content: memberContent})
	}
	return banEvents, nil
}

func createTemporaryPowerLevels(powerLevelContent *gomatrixserverlib.PowerLevelContent, userID string) fledglingEvent {
	eventPowerLevels := powerLevelContent.Events
	stateDefaultPowerLevel := powerLevelContent.StateDefault
	neededPowerLevel := stateDefaultPowerLevel
	for _, powerLevel := range eventPowerLevels {
		if powerLevel > neededPowerLevel {
			neededPowerLevel = powerLevel
		}
	}

	tempPowerLevelContent := &gomatrixserverlib.PowerLevelContent{}
	*tempPowerLevelContent = *powerLevelContent
	newUserPowerLevels := make(map[string]int64)
	for key, value := range powerLevelContent.Users {
		newUserPowerLevels[key] = value
	}
	tempPowerLevelContent.Users = newUserPowerLevels

	if val, ok := tempPowerLevelContent.Users[userID]; ok {
		if val < neededPowerLevel {
			tempPowerLevelContent.Users[userID] = neededPowerLevel
		}
	} else {
		if tempPowerLevelContent.UsersDefault < val {
			tempPowerLevelContent.UsersDefault = neededPowerLevel
		}
	}
	tempPowerLevelsEvent := fledglingEvent{
		Type:    gomatrixserverlib.MRoomPowerLevels,
		Content: tempPowerLevelContent,
	}
	return tempPowerLevelsEvent
}

func (r *Upgrader) sendHeaderedEvent(
	ctx context.Context,
	headeredEvent *gomatrixserverlib.HeaderedEvent,
) *api.PerformError {
	var inputs []api.InputRoomEvent
	inputs = append(inputs, api.InputRoomEvent{
		Kind:         api.KindNew,
		Event:        headeredEvent,
		Origin:       r.Cfg.Matrix.ServerName,
		SendAsServer: api.DoNotSendToOtherServers,
	})
	if err := api.SendInputRoomEvents(ctx, r.URSAPI, inputs, false); err != nil {
		return &api.PerformError{
			Msg: "api.SendInputRoomEvents failed",
		}
	}

	return nil
}

func (r *Upgrader) buildEvent(
	builder *gomatrixserverlib.EventBuilder,
	provider gomatrixserverlib.AuthEventProvider,
	evTime time.Time,
	roomVersion gomatrixserverlib.RoomVersion,
) (*gomatrixserverlib.Event, error) {
	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		return nil, err
	}
	refs, err := eventsNeeded.AuthEventReferences(provider)
	if err != nil {
		return nil, err
	}
	builder.AuthEvents = refs
	event, err := builder.Build(
		evTime, r.Cfg.Matrix.ServerName, r.Cfg.Matrix.KeyID,
		r.Cfg.Matrix.PrivateKey, roomVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot build event %s : Builder failed to build. %w", builder.Type, err)
	}
	return event, nil
}

func unmarshal(in []byte) map[string]interface{} {
	ret := make(map[string]interface{})
	err := json.Unmarshal(in, &ret)
	if err != nil {
		logrus.Fatalf("One of our own state events is not valid JSON: %v", err)
	}
	return ret
}
