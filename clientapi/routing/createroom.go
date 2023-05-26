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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	roomserverVersion "github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-createroom
type createRoomRequest struct {
	Invite                    []string                      `json:"invite"`
	Name                      string                        `json:"name"`
	Visibility                string                        `json:"visibility"`
	Topic                     string                        `json:"topic"`
	Preset                    string                        `json:"preset"`
	CreationContent           json.RawMessage               `json:"creation_content"`
	InitialState              []fledglingEvent              `json:"initial_state"`
	RoomAliasName             string                        `json:"room_alias_name"`
	RoomVersion               gomatrixserverlib.RoomVersion `json:"room_version"`
	PowerLevelContentOverride json.RawMessage               `json:"power_level_content_override"`
	IsDirect                  bool                          `json:"is_direct"`
}

const (
	presetPrivateChat        = "private_chat"
	presetTrustedPrivateChat = "trusted_private_chat"
	presetPublicChat         = "public_chat"
)

const (
	historyVisibilityShared = "shared"
	// TODO: These should be implemented once history visibility is implemented
	// historyVisibilityWorldReadable = "world_readable"
	// historyVisibilityInvited       = "invited"
)

func (r createRoomRequest) Validate() *util.JSONResponse {
	whitespace := "\t\n\x0b\x0c\r " // https://docs.python.org/2/library/string.html#string.whitespace
	// https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/handlers/room.py#L81
	// Synapse doesn't check for ':' but we will else it will break parsers badly which split things into 2 segments.
	if strings.ContainsAny(r.RoomAliasName, whitespace+":") {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("room_alias_name cannot contain whitespace or ':'"),
		}
	}
	for _, userID := range r.Invite {
		if _, err := spec.NewUserID(userID, true); err != nil {
			return &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("user id must be in the form @localpart:domain"),
			}
		}
	}
	switch r.Preset {
	case presetPrivateChat, presetTrustedPrivateChat, presetPublicChat, "":
	default:
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("preset must be any of 'private_chat', 'trusted_private_chat', 'public_chat'"),
		}
	}

	// Validate creation_content fields defined in the spec by marshalling the
	// creation_content map into bytes and then unmarshalling the bytes into
	// eventutil.CreateContent.

	creationContentBytes, err := json.Marshal(r.CreationContent)
	if err != nil {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("malformed creation_content"),
		}
	}

	var CreationContent gomatrixserverlib.CreateContent
	err = json.Unmarshal(creationContentBytes, &CreationContent)
	if err != nil {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("malformed creation_content"),
		}
	}

	return nil
}

// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-createroom
type createRoomResponse struct {
	RoomID    string `json:"room_id"`
	RoomAlias string `json:"room_alias,omitempty"` // in synapse not spec
}

// fledglingEvent is a helper representation of an event used when creating many events in succession.
type fledglingEvent struct {
	Type     string      `json:"type"`
	StateKey string      `json:"state_key"`
	Content  interface{} `json:"content"`
}

// CreateRoom implements /createRoom
func CreateRoom(
	req *http.Request, device *api.Device,
	cfg *config.ClientAPI,
	profileAPI api.ClientUserAPI, rsAPI roomserverAPI.ClientRoomserverAPI,
	asAPI appserviceAPI.AppServiceInternalAPI,
) util.JSONResponse {
	var createRequest createRoomRequest
	resErr := httputil.UnmarshalJSONRequest(req, &createRequest)
	if resErr != nil {
		return *resErr
	}
	if resErr = createRequest.Validate(); resErr != nil {
		return *resErr
	}
	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(err.Error()),
		}
	}
	return createRoom(req.Context(), createRequest, device, cfg, profileAPI, rsAPI, asAPI, evTime)
}

// createRoom implements /createRoom
// nolint: gocyclo
func createRoom(
	ctx context.Context,
	// TODO: remove dependency on createRoomRequest
	createRequest createRoomRequest, device *api.Device,
	cfg *config.ClientAPI,
	profileAPI api.ClientUserAPI, rsAPI roomserverAPI.ClientRoomserverAPI,
	asAPI appserviceAPI.AppServiceInternalAPI,
	evTime time.Time,
) util.JSONResponse {
	userID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("invalid userID")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if !cfg.Matrix.IsLocalServerName(userID.Domain()) {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden(fmt.Sprintf("User domain %q not configured locally", userID.Domain())),
		}
	}

	logger := util.GetLogger(ctx)

	// TODO (#267): Check room ID doesn't clash with an existing one, and we
	//              probably shouldn't be using pseudo-random strings, maybe GUIDs?
	roomID, err := spec.NewRoomID(fmt.Sprintf("!%s:%s", util.RandomString(16), userID.Domain()))
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("invalid roomID")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// Clobber keys: creator, room_version

	roomVersion := roomserverVersion.DefaultRoomVersion()
	if createRequest.RoomVersion != "" {
		candidateVersion := gomatrixserverlib.RoomVersion(createRequest.RoomVersion)
		_, roomVersionError := roomserverVersion.SupportedRoomVersion(candidateVersion)
		if roomVersionError != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.UnsupportedRoomVersion(roomVersionError.Error()),
			}
		}
		roomVersion = candidateVersion
	}

	logger.WithFields(log.Fields{
		"userID":      userID.String(),
		"roomID":      roomID.String(),
		"roomVersion": roomVersion,
	}).Info("Creating new room")

	profile, err := appserviceAPI.RetrieveUserProfile(ctx, userID.String(), asAPI, profileAPI)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("appserviceAPI.RetrieveUserProfile failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	userDisplayName := profile.DisplayName
	userAvatarURL := profile.AvatarURL

	keyID := cfg.Matrix.KeyID
	privateKey := cfg.Matrix.PrivateKey

	// TODO: Move this whole function to roomserver to start
	// HACK: asdklfjsdklfj

	// TODO: visibility/presets/raw initial state
	// TODO: Create room alias association
	// Make sure this doesn't fall into an application service's namespace though!

	createContent := map[string]interface{}{}
	if len(createRequest.CreationContent) > 0 {
		if err = json.Unmarshal(createRequest.CreationContent, &createContent); err != nil {
			util.GetLogger(ctx).WithError(err).Error("json.Unmarshal for creation_content failed")
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("invalid create content"),
			}
		}
	}
	createContent["creator"] = userID.String()
	createContent["room_version"] = roomVersion
	powerLevelContent := eventutil.InitialPowerLevelsContent(userID.String())
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
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("malformed power_level_content_override"),
			}
		}
	}

	var guestsCanJoin bool
	switch createRequest.Preset {
	case presetPrivateChat:
		joinRuleContent.JoinRule = spec.Invite
		historyVisibilityContent.HistoryVisibility = historyVisibilityShared
		guestsCanJoin = true
	case presetTrustedPrivateChat:
		joinRuleContent.JoinRule = spec.Invite
		historyVisibilityContent.HistoryVisibility = historyVisibilityShared
		for _, invitee := range createRequest.Invite {
			powerLevelContent.Users[invitee] = 100
		}
		guestsCanJoin = true
	case presetPublicChat:
		joinRuleContent.JoinRule = spec.Public
		historyVisibilityContent.HistoryVisibility = historyVisibilityShared
	}

	createEvent := fledglingEvent{
		Type:    spec.MRoomCreate,
		Content: createContent,
	}
	powerLevelEvent := fledglingEvent{
		Type:    spec.MRoomPowerLevels,
		Content: powerLevelContent,
	}
	joinRuleEvent := fledglingEvent{
		Type:    spec.MRoomJoinRules,
		Content: joinRuleContent,
	}
	historyVisibilityEvent := fledglingEvent{
		Type:    spec.MRoomHistoryVisibility,
		Content: historyVisibilityContent,
	}
	membershipEvent := fledglingEvent{
		Type:     spec.MRoomMember,
		StateKey: userID.String(),
		Content: gomatrixserverlib.MemberContent{
			Membership:  spec.Join,
			DisplayName: userDisplayName,
			AvatarURL:   userAvatarURL,
		},
	}

	var nameEvent *fledglingEvent
	var topicEvent *fledglingEvent
	var guestAccessEvent *fledglingEvent
	var aliasEvent *fledglingEvent

	if createRequest.Name != "" {
		nameEvent = &fledglingEvent{
			Type: spec.MRoomName,
			Content: eventutil.NameContent{
				Name: createRequest.Name,
			},
		}
	}

	if createRequest.Topic != "" {
		topicEvent = &fledglingEvent{
			Type: spec.MRoomTopic,
			Content: eventutil.TopicContent{
				Topic: createRequest.Topic,
			},
		}
	}

	if guestsCanJoin {
		guestAccessEvent = &fledglingEvent{
			Type: spec.MRoomGuestAccess,
			Content: eventutil.GuestAccessContent{
				GuestAccess: "can_join",
			},
		}
	}

	var roomAlias string
	if createRequest.RoomAliasName != "" {
		roomAlias = fmt.Sprintf("#%s:%s", createRequest.RoomAliasName, userID.Domain())
		// check it's free TODO: This races but is better than nothing
		hasAliasReq := roomserverAPI.GetRoomIDForAliasRequest{
			Alias:              roomAlias,
			IncludeAppservices: false,
		}

		var aliasResp roomserverAPI.GetRoomIDForAliasResponse
		err = rsAPI.GetRoomIDForAlias(ctx, &hasAliasReq, &aliasResp)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("aliasAPI.GetRoomIDForAlias failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		if aliasResp.RoomID != "" {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.RoomInUse("Room ID already exists."),
			}
		}

		aliasEvent = &fledglingEvent{
			Type: spec.MRoomCanonicalAlias,
			Content: eventutil.CanonicalAlias{
				Alias: roomAlias,
			},
		}
	}

	var initialStateEvents []fledglingEvent
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
	eventsToMake := []fledglingEvent{
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

	verImpl, err := gomatrixserverlib.GetRoomVersion(roomVersion)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("unknown room version"),
		}
	}

	var builtEvents []*types.HeaderedEvent
	authEvents := gomatrixserverlib.NewAuthEvents(nil)
	for i, e := range eventsToMake {
		depth := i + 1 // depth starts at 1

		builder := verImpl.NewEventBuilderFromProtoEvent(&gomatrixserverlib.ProtoEvent{
			Sender:   userID.String(),
			RoomID:   roomID.String(),
			Type:     e.Type,
			StateKey: &e.StateKey,
			Depth:    int64(depth),
		})
		err = builder.SetContent(e.Content)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("builder.SetContent failed")
			return util.JSONResponse{
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
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		ev, err = builder.Build(evTime, userID.Domain(), keyID, privateKey)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("buildEvent failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		if err = gomatrixserverlib.Allowed(ev, &authEvents); err != nil {
			util.GetLogger(ctx).WithError(err).Error("gomatrixserverlib.Allowed failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		// Add the event to the list of auth events
		builtEvents = append(builtEvents, &types.HeaderedEvent{PDU: ev})
		err = authEvents.AddEvent(ev)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("authEvents.AddEvent failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
	}

	inputs := make([]roomserverAPI.InputRoomEvent, 0, len(builtEvents))
	for _, event := range builtEvents {
		inputs = append(inputs, roomserverAPI.InputRoomEvent{
			Kind:         roomserverAPI.KindNew,
			Event:        event,
			Origin:       userID.Domain(),
			SendAsServer: roomserverAPI.DoNotSendToOtherServers,
		})
	}
	if err = roomserverAPI.SendInputRoomEvents(ctx, rsAPI, userID.Domain(), inputs, false); err != nil {
		util.GetLogger(ctx).WithError(err).Error("roomserverAPI.SendInputRoomEvents failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// TODO(#269): Reserve room alias while we create the room. This stops us
	// from creating the room but still failing due to the alias having already
	// been taken.
	if roomAlias != "" {
		aliasReq := roomserverAPI.SetRoomAliasRequest{
			Alias:  roomAlias,
			RoomID: roomID.String(),
			UserID: userID.String(),
		}

		var aliasResp roomserverAPI.SetRoomAliasResponse
		err = rsAPI.SetRoomAlias(ctx, &aliasReq, &aliasResp)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("aliasAPI.SetRoomAlias failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		if aliasResp.AliasExists {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.RoomInUse("Room alias already exists."),
			}
		}
	}

	// If this is a direct message then we should invite the participants.
	if len(createRequest.Invite) > 0 {
		// Build some stripped state for the invite.
		var globalStrippedState []fclient.InviteV2StrippedState
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
					fclient.NewInviteV2StrippedState(ev),
				)
			}
		}

		// Process the invites.
		var inviteEvent *types.HeaderedEvent
		for _, invitee := range createRequest.Invite {
			// Build the invite event.
			inviteEvent, err = buildMembershipEventDirect(
				ctx, invitee, "", userDisplayName, userAvatarURL, userID.String(), userID.Domain(), spec.Invite,
				roomID.String(), createRequest.IsDirect, keyID, privateKey, evTime, rsAPI,
			)
			if err != nil {
				util.GetLogger(ctx).WithError(err).Error("buildMembershipEvent failed")
				continue
			}
			inviteStrippedState := append(
				globalStrippedState,
				fclient.NewInviteV2StrippedState(inviteEvent.PDU),
			)
			// Send the invite event to the roomserver.
			event := inviteEvent
			err = rsAPI.PerformInvite(ctx, &roomserverAPI.PerformInviteRequest{
				Event:           event,
				InviteRoomState: inviteStrippedState,
				RoomVersion:     event.Version(),
				SendAsServer:    string(userID.Domain()),
			})
			switch e := err.(type) {
			case roomserverAPI.ErrInvalidID:
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.Unknown(e.Error()),
				}
			case roomserverAPI.ErrNotAllowed:
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden(e.Error()),
				}
			case nil:
			default:
				util.GetLogger(ctx).WithError(err).Error("PerformInvite failed")
				sentry.CaptureException(err)
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}
		}
	}

	if createRequest.Visibility == spec.Public {
		// expose this room in the published room list
		if err = rsAPI.PerformPublish(ctx, &roomserverAPI.PerformPublishRequest{
			RoomID:     roomID.String(),
			Visibility: spec.Public,
		}); err != nil {
			util.GetLogger(ctx).WithError(err).Error("failed to publish room")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
	}

	response := createRoomResponse{
		RoomID:    roomID.String(),
		RoomAlias: roomAlias,
	}

	return util.JSONResponse{
		Code: 200,
		JSON: response,
	}
}
