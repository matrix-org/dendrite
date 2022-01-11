// Copyright 2017 Vector Creations Ltd
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

package routing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	accounts "github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type upgradeRoomRequest struct {
	NewVersion string `json:"new_version"`
}

type upgradeRoomResponse struct {
	ReplacementRoom string `json:"replacement_room"`
}

func UpgradeRoom(
	req *http.Request, device *userapi.Device,
	cfg *config.ClientAPI,
	roomID string, accountDB accounts.Database,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
) util.JSONResponse {
	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
	}

	var r upgradeRoomRequest
	if rErr := httputil.UnmarshalJSONRequest(req, &r); rErr != nil {
		return *rErr
	}

	// Validate that the room version is supported
	if _, err := version.SupportedRoomVersion(gomatrixserverlib.RoomVersion(r.NewVersion)); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnsupportedRoomVersion("This server does not support that room version"),
		}
	}

	// Return an immediate error if the room does not exist
	verReq := roomserverAPI.QueryRoomVersionForRoomRequest{RoomID: roomID}
	verRes := roomserverAPI.QueryRoomVersionForRoomResponse{}
	if err := rsAPI.QueryRoomVersionForRoom(req.Context(), &verReq, &verRes); err != nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	}

	// 1. Check if the user is authorized to actually perform the upgrade (can send m.room.tombstone)
	if err := userIsAuthorized(req, device, roomID, rsAPI); err != nil {
		return *err
	}

	oldCreateEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomCreate,
		StateKey:  "",
	})
	oldPowerLevelsEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomPowerLevels,
		StateKey:  "",
	})
	oldJoinRulesEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomJoinRules,
		StateKey:  "",
	})
	oldHistoryVisibilityEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomHistoryVisibility,
		StateKey:  "",
	})
	oldNameEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomName,
		StateKey:  "",
	})
	oldTopicEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomTopic,
		StateKey:  "",
	})
	oldGuestAccessEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomGuestAccess,
		StateKey:  "",
	})
	oldAvatarEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomAvatar,
		StateKey:  "",
	})
	oldEncryptionEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomEncryption,
		StateKey:  "",
	})
	oldServerAclEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: "m.room.server_acl",
		StateKey:  "",
	})
	// Not in the spec, but needed for sytest compatablility
	oldRelatedGroupsEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: "m.room.related_groups",
		StateKey:  "",
	})
	oldCanonicalAliasEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomCanonicalAlias,
		StateKey:  "",
	})

	// TODO (#267): Check room ID doesn't clash with an existing one, and we
	//              probably shouldn't be using pseudo-random strings, maybe GUIDs?
	newRoomID := fmt.Sprintf("!%s:%s", util.RandomString(16), cfg.Matrix.ServerName)
	userID := device.UserID
	profile, err := appserviceAPI.RetrieveUserProfile(req.Context(), userID, asAPI, accountDB)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("appserviceAPI.RetrieveUserProfile failed")
		return jsonerror.InternalServerError()
	}

	// Make the tombstone event
	tombstoneEvent, resErr := makeTombstoneEvent(req, device, cfg, evTime, roomID, newRoomID, accountDB, rsAPI)
	if err != nil {
		return *resErr
	}

	newCreateContent := map[string]interface{}{
		"creator":      userID,
		"room_version": r.NewVersion, // TODO: change struct to single var?
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

	membershipEvent := fledglingEvent{
		Type:     gomatrixserverlib.MRoomMember,
		StateKey: userID,
		Content: gomatrixserverlib.MemberContent{
			Membership:  gomatrixserverlib.Join,
			DisplayName: profile.DisplayName,
			AvatarURL:   profile.AvatarURL,
		},
	}

	powerLevelContent, err := oldPowerLevelsEvent.PowerLevels()
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("powerLevel event was not actually a power level event")
		return jsonerror.InternalServerError()
	}

	newPowerLevelsEvent := fledglingEvent{
		Type:    gomatrixserverlib.MRoomPowerLevels,
		Content: powerLevelContent,
	}

	//create temporary power level event that elevates upgrading user's prvileges to create every copied state event
	tempPowerLevelsEvent := createTemporaryPowerLevels(powerLevelContent, userID)

	joinRulesContent, err := oldJoinRulesEvent.JoinRule()
	newJoinRulesEvent := fledglingEvent{
		Type: gomatrixserverlib.MRoomJoinRules,
		Content: map[string]interface{}{
			"join_rule": joinRulesContent,
		},
	}

	historyVisibilityContent, err := oldHistoryVisibilityEvent.HistoryVisibility()
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
	banEvents, err := getBanEvents(req, roomID, rsAPI)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryCurrentState failed")
		return jsonerror.InternalServerError()
	} else {
		eventsToMake = append(eventsToMake, banEvents...)
	}
	eventsToMake = append(eventsToMake, newPowerLevelsEvent)

	// 5. Send the tombstone event to the old room (must do this before we set the new canonical_alias)
	resErr = sendHeaderedEvent(req, cfg, rsAPI, tombstoneEvent)
	if resErr != nil {
		return *resErr
	}

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
			util.GetLogger(req.Context()).WithError(err).Error("builder.SetContent failed")
			return jsonerror.InternalServerError()
		}
		if i > 0 {
			builder.PrevEvents = []gomatrixserverlib.EventReference{builtEvents[i-1].EventReference()}
		}
		var event *gomatrixserverlib.Event
		event, err = buildEvent(&builder, &authEvents, cfg, evTime, gomatrixserverlib.RoomVersion(r.NewVersion))
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("buildEvent failed")
			return jsonerror.InternalServerError()
		}

		if err = gomatrixserverlib.Allowed(event, &authEvents); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.Allowed failed")
			return jsonerror.InternalServerError()
		}

		// Add the event to the list of auth events
		builtEvents = append(builtEvents, event.Headered(gomatrixserverlib.RoomVersion(r.NewVersion)))
		err = authEvents.AddEvent(event)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("authEvents.AddEvent failed")
			return jsonerror.InternalServerError()
		}
	}

	inputs := make([]roomserverAPI.InputRoomEvent, 0, len(builtEvents))
	for _, event := range builtEvents {
		inputs = append(inputs, roomserverAPI.InputRoomEvent{
			Kind:         roomserverAPI.KindNew,
			Event:        event,
			Origin:       cfg.Matrix.ServerName,
			SendAsServer: roomserverAPI.DoNotSendToOtherServers,
		})
	}
	if err = roomserverAPI.SendInputRoomEvents(req.Context(), rsAPI, inputs, false); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("roomserverAPI.SendInputRoomEvents failed")
		return jsonerror.InternalServerError()
	}

	// check if the old room was published
	var pubQueryRes roomserverAPI.QueryPublishedRoomsResponse
	err = rsAPI.QueryPublishedRooms(req.Context(), &roomserverAPI.QueryPublishedRoomsRequest{
		RoomID: roomID,
	}, &pubQueryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryPublishedRooms failed")
		return jsonerror.InternalServerError()
	}

	// if the old room is published (was public), publish the new room
	if len(pubQueryRes.RoomIDs) == 1 {
		publishNewRoom(req, rsAPI, roomID, newRoomID)
	}

	// Clear the old canonical alias event in the old room
	emptyCanonicalAliasEvent, resErr := makeHeaderedEvent(req, device, cfg, evTime, roomID, accountDB, rsAPI, fledglingEvent{
		Type:    gomatrixserverlib.MRoomCanonicalAlias,
		Content: map[string]interface{}{},
	})
	if resErr != nil {
		if resErr.Code == http.StatusForbidden {
			util.GetLogger(req.Context()).WithField(logrus.ErrorKey, resErr).Warn("UpgradeRoom: Could not set empty canonical alias event in old room")
		} else {
			return *resErr
		}
	} else {
		if resErr = sendHeaderedEvent(req, cfg, rsAPI, emptyCanonicalAliasEvent); resErr != nil {
			return *resErr
		}
	}

	// 4. Move local aliases to the new room
	if resErr = moveLocalAliases(req, roomID, newRoomID, userID, rsAPI); resErr != nil {
		return *resErr
	}

	// 6. Restrict power levels in the old room
	if resErr = restrictOldRoomPowerLevels(req, device, cfg, evTime, roomID, accountDB, rsAPI, *powerLevelContent); resErr != nil {
		return *resErr
	}

	return util.JSONResponse{
		Code: 200,
		JSON: upgradeRoomResponse{
			ReplacementRoom: newRoomID,
		},
	}
}

func publishNewRoom(
	req *http.Request,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	oldRoomID, newRoomID string,
) {

	// expose this room in the published room list
	var pubNewRoomRes roomserverAPI.PerformPublishResponse
	rsAPI.PerformPublish(req.Context(), &roomserverAPI.PerformPublishRequest{
		RoomID:     newRoomID,
		Visibility: "public",
	}, &pubNewRoomRes)
	if pubNewRoomRes.Error != nil {
		// treat as non-fatal since the room is already made by this point
		util.GetLogger(req.Context()).WithError(pubNewRoomRes.Error).Error("failed to visibility:public")
	}

	var unpubOldRoomRes roomserverAPI.PerformPublishResponse
	// remove the old room from the published room list
	rsAPI.PerformPublish(req.Context(), &roomserverAPI.PerformPublishRequest{
		RoomID:     oldRoomID,
		Visibility: "private",
	}, &unpubOldRoomRes)
	if unpubOldRoomRes.Error != nil {
		// treat as non-fatal since the room is already made by this point
		util.GetLogger(req.Context()).WithError(unpubOldRoomRes.Error).Error("failed to visibility:private")
	}
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

func getBanEvents(req *http.Request, roomID string, rsAPI roomserverAPI.RoomserverInternalAPI) ([]fledglingEvent, error) {
	var err error
	banEvents := []fledglingEvent{}

	roomMemberReq := roomserverAPI.QueryCurrentStateRequest{RoomID: roomID, AllowWildcards: true, StateTuples: []gomatrixserverlib.StateKeyTuple{
		{EventType: gomatrixserverlib.MRoomMember, StateKey: "*"},
	}}
	roomMemberRes := roomserverAPI.QueryCurrentStateResponse{}
	if err = rsAPI.QueryCurrentState(req.Context(), &roomMemberReq, &roomMemberRes); err != nil {
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

func moveLocalAliases(req *http.Request,
	roomID, newRoomID, userID string,
	rsAPI roomserverAPI.RoomserverInternalAPI) *util.JSONResponse {
	var err error
	internalServerError := jsonerror.InternalServerError()

	aliasReq := roomserverAPI.GetAliasesForRoomIDRequest{RoomID: roomID}
	aliasRes := roomserverAPI.GetAliasesForRoomIDResponse{}
	if err := rsAPI.GetAliasesForRoomID(req.Context(), &aliasReq, &aliasRes); err != nil {
		return &internalServerError
	}

	for _, alias := range aliasRes.Aliases {
		removeAliasReq := roomserverAPI.RemoveRoomAliasRequest{UserID: userID, Alias: alias}
		removeAliasRes := roomserverAPI.RemoveRoomAliasResponse{}
		if err = rsAPI.RemoveRoomAlias(req.Context(), &removeAliasReq, &removeAliasRes); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("roomserverAPI.RemoveRoomAlias failed")
			return &internalServerError
		}

		setAliasReq := roomserverAPI.SetRoomAliasRequest{UserID: userID, Alias: alias, RoomID: newRoomID}
		setAliasRes := roomserverAPI.SetRoomAliasResponse{}
		if err = rsAPI.SetRoomAlias(req.Context(), &setAliasReq, &setAliasRes); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("roomserverAPI.SetRoomAlias failed")
			return &internalServerError
		}
	}
	return nil
}

func restrictOldRoomPowerLevels(req *http.Request, device *userapi.Device,
	cfg *config.ClientAPI, evTime time.Time,
	roomID string, accountDB accounts.Database,
	rsAPI roomserverAPI.RoomserverInternalAPI, powerLevelContent gomatrixserverlib.PowerLevelContent) *util.JSONResponse {
	restrictedPowerLevelContent := &gomatrixserverlib.PowerLevelContent{}
	*restrictedPowerLevelContent = powerLevelContent

	restrictedDefaultPowerLevel := int64(50)
	if restrictedPowerLevelContent.UsersDefault+1 > restrictedDefaultPowerLevel {
		restrictedDefaultPowerLevel = restrictedPowerLevelContent.UsersDefault + 1
	}
	restrictedPowerLevelContent.EventsDefault = restrictedDefaultPowerLevel
	restrictedPowerLevelContent.Invite = restrictedDefaultPowerLevel

	restrictedPowerLevelsHeadered, resErr := makeHeaderedEvent(req, device, cfg, evTime, roomID, accountDB, rsAPI, fledglingEvent{
		Type:    gomatrixserverlib.MRoomPowerLevels,
		Content: restrictedPowerLevelContent,
	})
	if resErr != nil {
		if resErr.Code == http.StatusForbidden {
			util.GetLogger(req.Context()).WithField(logrus.ErrorKey, resErr).Warn("UpgradeRoom: Could not restrict power levels in old room")
		} else {
			return resErr
		}
	} else {
		if resErr = sendHeaderedEvent(req, cfg, rsAPI, restrictedPowerLevelsHeadered); resErr != nil {
			return resErr
		}
	}
	return nil
}

func userIsAuthorized(
	req *http.Request, device *userapi.Device,
	roomID string,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) *util.JSONResponse {
	plEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomPowerLevels,
		StateKey:  "",
	})
	if plEvent == nil {
		return &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You don't have permission to upgrade this room, no power_levels event in this room."),
		}
	}

	pl, err := plEvent.PowerLevels()
	if err != nil {
		return &util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("The power_levels event for this room is malformed so auth checks cannot be performed."),
		}
	}

	// Check for power level required to send tombstone event (marks the curren room as obsolete),
	// if not found, use the StateDefault power level
	plToUpgrade, ok := pl.Events["m.room.tombstone"]
	if !ok {
		plToUpgrade = pl.StateDefault
	}

	allowedToUpgrade := pl.UserLevel(device.UserID) >= plToUpgrade
	if !allowedToUpgrade {
		return &util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("You don't have permission to upgrade the room, power level too low."),
		}
	}

	return nil
}

func makeHeaderedEvent(req *http.Request, device *userapi.Device,
	cfg *config.ClientAPI, evTime time.Time,
	roomID string, accountDB accounts.Database,
	rsAPI roomserverAPI.RoomserverInternalAPI, event fledglingEvent) (*gomatrixserverlib.HeaderedEvent, *util.JSONResponse) {

	builder := gomatrixserverlib.EventBuilder{
		Sender:   device.UserID,
		RoomID:   roomID,
		Type:     event.Type,
		StateKey: &event.StateKey,
	}
	err := builder.SetContent(event.Content)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("builder.SetContent failed")
		resErr := jsonerror.InternalServerError()
		return nil, &resErr
	}
	var queryRes api.QueryLatestEventsAndStateResponse
	headeredEvent, err := eventutil.QueryAndBuildEvent(req.Context(), &builder, cfg.Matrix, evTime, rsAPI, &queryRes)
	if err == eventutil.ErrRoomNoExists {
		return nil, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	} else if e, ok := err.(gomatrixserverlib.BadJSONError); ok {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(e.Error()),
		}
	} else if e, ok := err.(gomatrixserverlib.EventValidationError); ok {
		if e.Code == gomatrixserverlib.EventValidationTooLarge {
			return nil, &util.JSONResponse{
				Code: http.StatusRequestEntityTooLarge,
				JSON: jsonerror.BadJSON(e.Error()),
			}
		}
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(e.Error()),
		}
	} else if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("eventutil.BuildEvent failed")
		resErr := jsonerror.InternalServerError()
		return nil, &resErr
	}

	// check to see if this user can perform this operation
	stateEvents := make([]*gomatrixserverlib.Event, len(queryRes.StateEvents))
	for i := range queryRes.StateEvents {
		stateEvents[i] = queryRes.StateEvents[i].Event
	}
	provider := gomatrixserverlib.NewAuthEvents(stateEvents)
	if err = gomatrixserverlib.Allowed(headeredEvent.Event, &provider); err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden(err.Error()), // TODO: Is this error string comprehensible to the client?
		}
	}

	return headeredEvent, nil

}

func makeTombstoneEvent(
	req *http.Request, device *userapi.Device,
	cfg *config.ClientAPI, evTime time.Time,
	roomID, newRoomID string, accountDB accounts.Database,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) (*gomatrixserverlib.HeaderedEvent, *util.JSONResponse) {
	content := map[string]interface{}{
		"body":             "This room has been replaced",
		"replacement_room": newRoomID,
	}
	event := fledglingEvent{
		Type:    "m.room.tombstone",
		Content: content,
	}
	return makeHeaderedEvent(req, device, cfg, evTime, roomID, accountDB, rsAPI, event)

}

func sendHeaderedEvent(
	req *http.Request,
	cfg *config.ClientAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	headeredEvent *gomatrixserverlib.HeaderedEvent,
) *util.JSONResponse {
	var inputs []roomserverAPI.InputRoomEvent
	inputs = append(inputs, roomserverAPI.InputRoomEvent{
		Kind:         roomserverAPI.KindNew,
		Event:        headeredEvent,
		Origin:       cfg.Matrix.ServerName,
		SendAsServer: roomserverAPI.DoNotSendToOtherServers,
	})
	if err := roomserverAPI.SendInputRoomEvents(req.Context(), rsAPI, inputs, false); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("roomserverAPI.SendInputRoomEvents failed")
		resErr := jsonerror.InternalServerError()
		return &resErr
	}

	return nil
}

func unmarshal(in []byte) map[string]interface{} {
	ret := make(map[string]interface{})
	err := json.Unmarshal(in, &ret)
	if err != nil {
		logrus.Fatalf("One of our own state events is not valid JSON: %v", err)
	}
	return ret
}
