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

	// Get the existing room state for the old room.
	oldRoomReq := &api.QueryLatestEventsAndStateRequest{
		RoomID: roomID,
	}
	oldRoomRes := &api.QueryLatestEventsAndStateResponse{}
	if err := r.URSAPI.QueryLatestEventsAndState(ctx, oldRoomReq, oldRoomRes); err != nil {
		return "", &api.PerformError{
			Msg: fmt.Sprintf("Failed to get latest state: %s", err),
		}
	}

	// Make the tombstone event
	tombstoneEvent, pErr := r.makeTombstoneEvent(ctx, evTime, userID, roomID, newRoomID)
	if pErr != nil {
		return "", pErr
	}

	// Generate the initial events we need to send into the new room. This includes copied state events and bans
	// as well as the power level events needed to set up the room
	eventsToMake, pErr := r.generateInitialEvents(ctx, oldRoomRes, userID, roomID, string(req.RoomVersion), tombstoneEvent)
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
	if pErr = r.clearOldCanonicalAliasEvent(ctx, oldRoomRes, evTime, userID, roomID); pErr != nil {
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
	restrictedPowerLevelContent, pErr := r.getRoomPowerLevels(ctx, roomID)
	if pErr != nil {
		return pErr
	}

	// From: https://spec.matrix.org/v1.2/client-server-api/#server-behaviour-16
	// If possible, the power levels in the old room should also be modified to
	// prevent sending of events and inviting new users. For example, setting
	// events_default and invite to the greater of 50 and users_default + 1.
	restrictedDefaultPowerLevel := int64(50)
	if restrictedPowerLevelContent.UsersDefault+1 > restrictedDefaultPowerLevel {
		restrictedDefaultPowerLevel = restrictedPowerLevelContent.UsersDefault + 1
	}
	restrictedPowerLevelContent.EventsDefault = restrictedDefaultPowerLevel
	restrictedPowerLevelContent.Invite = restrictedDefaultPowerLevel

	restrictedPowerLevelsHeadered, resErr := r.makeHeaderedEvent(ctx, evTime, userID, roomID, fledglingEvent{
		Type:     gomatrixserverlib.MRoomPowerLevels,
		StateKey: "",
		Content:  restrictedPowerLevelContent,
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

func (r *Upgrader) clearOldCanonicalAliasEvent(ctx context.Context, oldRoom *api.QueryLatestEventsAndStateResponse, evTime time.Time, userID, roomID string) *api.PerformError {
	for _, event := range oldRoom.StateEvents {
		if event.Type() != gomatrixserverlib.MRoomCanonicalAlias || !event.StateKeyEquals("") {
			continue
		}
		var aliasContent struct {
			Alias      string   `json:"alias"`
			AltAliases []string `json:"alt_aliases"`
		}
		if err := json.Unmarshal(event.Content(), &aliasContent); err != nil {
			return &api.PerformError{
				Msg: fmt.Sprintf("Failed to unmarshal canonical aliases: %s", err),
			}
		}
		if aliasContent.Alias == "" && len(aliasContent.AltAliases) == 0 {
			// There are no canonical aliases to clear, therefore do nothing.
			return nil
		}
	}

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
		publishNewRoomAndUnpublishOldRoom(ctx, r.URSAPI, roomID, newRoomID)
	}
	return nil
}

func publishNewRoomAndUnpublishOldRoom(
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
	// Check for power level required to send tombstone event (marks the current room as obsolete),
	// if not found, use the StateDefault power level
	return pl.UserLevel(userID) >= pl.EventLevel("m.room.tombstone", true)
}

// nolint:gocyclo
func (r *Upgrader) generateInitialEvents(ctx context.Context, oldRoom *api.QueryLatestEventsAndStateResponse, userID, roomID, newVersion string, tombstoneEvent *gomatrixserverlib.HeaderedEvent) ([]fledglingEvent, *api.PerformError) {
	state := make(map[gomatrixserverlib.StateKeyTuple]*gomatrixserverlib.HeaderedEvent, len(oldRoom.StateEvents))
	for _, event := range oldRoom.StateEvents {
		if event.StateKey() == nil {
			// This shouldn't ever happen, but better to be safe than sorry.
			continue
		}
		if event.Type() == gomatrixserverlib.MRoomMember && !event.StateKeyEquals(userID) {
			// With the exception of bans and invites which we do want to copy, we
			// should ignore membership events that aren't our own, as event auth will
			// prevent us from being able to create membership events on behalf of other
			// users anyway unless they are invites or bans.
			membership, err := event.Membership()
			if err != nil {
				continue
			}
			switch membership {
			case gomatrixserverlib.Ban:
			case gomatrixserverlib.Invite:
			default:
				continue
			}
		}
		state[gomatrixserverlib.StateKeyTuple{EventType: event.Type(), StateKey: *event.StateKey()}] = event
	}

	// The following events are ones that we are going to override manually
	// in the following section.
	override := map[gomatrixserverlib.StateKeyTuple]struct{}{
		{EventType: gomatrixserverlib.MRoomCreate, StateKey: ""}:      {},
		{EventType: gomatrixserverlib.MRoomMember, StateKey: userID}:  {},
		{EventType: gomatrixserverlib.MRoomPowerLevels, StateKey: ""}: {},
		{EventType: gomatrixserverlib.MRoomJoinRules, StateKey: ""}:   {},
	}

	// The overridden events are essential events that must be present in the
	// old room state. Check that they are there.
	for tuple := range override {
		if _, ok := state[tuple]; !ok {
			return nil, &api.PerformError{
				Msg: fmt.Sprintf("Essential event of type %q state key %q is missing", tuple.EventType, tuple.StateKey),
			}
		}
	}

	oldCreateEvent := state[gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomCreate, StateKey: ""}]
	oldMembershipEvent := state[gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomMember, StateKey: userID}]
	oldPowerLevelsEvent := state[gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomPowerLevels, StateKey: ""}]
	oldJoinRulesEvent := state[gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomJoinRules, StateKey: ""}]

	// Create the new room create event. Using a map here instead of CreateContent
	// means that we preserve any other interesting fields that might be present
	// in the create event (such as for the room types MSC).
	newCreateContent := map[string]interface{}{}
	_ = json.Unmarshal(oldCreateEvent.Content(), &newCreateContent)
	newCreateContent["creator"] = userID
	newCreateContent["room_version"] = newVersion
	newCreateContent["predecessor"] = gomatrixserverlib.PreviousRoom{
		EventID: tombstoneEvent.EventID(),
		RoomID:  roomID,
	}
	newCreateEvent := fledglingEvent{
		Type:     gomatrixserverlib.MRoomCreate,
		StateKey: "",
		Content:  newCreateContent,
	}

	// Now create the new membership event. Same rules apply as above, so
	// that we preserve fields we don't otherwise know about. We'll always
	// set the membership to join though, because that is necessary to auth
	// the events after it.
	newMembershipContent := map[string]interface{}{}
	_ = json.Unmarshal(oldMembershipEvent.Content(), &newMembershipContent)
	newMembershipContent["membership"] = gomatrixserverlib.Join
	newMembershipEvent := fledglingEvent{
		Type:     gomatrixserverlib.MRoomMember,
		StateKey: userID,
		Content:  newMembershipContent,
	}

	// We might need to temporarily give ourselves a higher power level
	// than we had in the old room in order to be able to send all of
	// the relevant state events. This function will return whether we
	// had to override the power level events or not — if we did, we
	// need to send the original power levels again later on.
	powerLevelContent, err := oldPowerLevelsEvent.PowerLevels()
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error()
		return nil, &api.PerformError{
			Msg: "Power level event content was invalid",
		}
	}
	tempPowerLevelsEvent, powerLevelsOverridden := createTemporaryPowerLevels(powerLevelContent, userID)

	// Now do the join rules event, same as the create and membership
	// events. We'll set a sane default of "invite" so that if the
	// existing join rules contains garbage, the room can still be
	// upgraded.
	newJoinRulesContent := map[string]interface{}{
		"join_rule": gomatrixserverlib.Invite, // sane default
	}
	_ = json.Unmarshal(oldJoinRulesEvent.Content(), &newJoinRulesContent)
	newJoinRulesEvent := fledglingEvent{
		Type:     gomatrixserverlib.MRoomJoinRules,
		StateKey: "",
		Content:  newJoinRulesContent,
	}

	eventsToMake := make([]fledglingEvent, 0, len(state))
	eventsToMake = append(
		eventsToMake, newCreateEvent, newMembershipEvent,
		tempPowerLevelsEvent, newJoinRulesEvent,
	)

	// For some reason Sytest expects there to be a guest access event.
	// Create one if it doesn't exist.
	if _, ok := state[gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomGuestAccess, StateKey: ""}]; !ok {
		eventsToMake = append(eventsToMake, fledglingEvent{
			Type: gomatrixserverlib.MRoomGuestAccess,
			Content: map[string]string{
				"guest_access": "forbidden",
			},
		})
	}

	// Duplicate all of the old state events into the new room.
	for tuple, event := range state {
		if _, ok := override[tuple]; ok {
			// Don't duplicate events we have overridden already. They
			// are already in `eventsToMake`.
			continue
		}
		newEvent := fledglingEvent{
			Type:     tuple.EventType,
			StateKey: tuple.StateKey,
		}
		if err = json.Unmarshal(event.Content(), &newEvent.Content); err != nil {
			logrus.WithError(err).Error("Failed to unmarshal old event")
			continue
		}
		eventsToMake = append(eventsToMake, newEvent)
	}

	// If we sent a temporary power level event into the room before,
	// override that now by restoring the original power levels.
	if powerLevelsOverridden {
		eventsToMake = append(eventsToMake, fledglingEvent{
			Type:    gomatrixserverlib.MRoomPowerLevels,
			Content: powerLevelContent,
		})
	}
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

func createTemporaryPowerLevels(powerLevelContent *gomatrixserverlib.PowerLevelContent, userID string) (fledglingEvent, bool) {
	// Work out what power level we need in order to be able to send events
	// of all types into the room.
	neededPowerLevel := powerLevelContent.StateDefault
	for _, powerLevel := range powerLevelContent.Events {
		if powerLevel > neededPowerLevel {
			neededPowerLevel = powerLevel
		}
	}

	// Make a copy of the existing power level content.
	tempPowerLevelContent := *powerLevelContent
	powerLevelsOverridden := false

	// At this point, the "Users", "Events" and "Notifications" keys are all
	// pointing to the map of the original PL content, so we will specifically
	// override the users map with a new one and duplicate the values deeply,
	// so that we can modify them without modifying the original.
	tempPowerLevelContent.Users = make(map[string]int64, len(powerLevelContent.Users))
	for key, value := range powerLevelContent.Users {
		tempPowerLevelContent.Users[key] = value
	}

	// If the user who is upgrading the room doesn't already have sufficient
	// power, then elevate their power levels.
	if tempPowerLevelContent.UserLevel(userID) < neededPowerLevel {
		tempPowerLevelContent.Users[userID] = neededPowerLevel
		powerLevelsOverridden = true
	}

	// Then return the temporary power levels event.
	return fledglingEvent{
		Type:    gomatrixserverlib.MRoomPowerLevels,
		Content: tempPowerLevelContent,
	}, powerLevelsOverridden
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
