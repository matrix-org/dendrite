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
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-createroom
type createRoomRequest struct {
	Invite          []string               `json:"invite"`
	Name            string                 `json:"name"`
	Visibility      string                 `json:"visibility"`
	Topic           string                 `json:"topic"`
	Preset          string                 `json:"preset"`
	CreationContent map[string]interface{} `json:"creation_content"`
	InitialState    []fledglingEvent       `json:"initial_state"`
	RoomAliasName   string                 `json:"room_alias_name"`
	GuestCanJoin    bool                   `json:"guest_can_join"`
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
			JSON: jsonerror.BadJSON("room_alias_name cannot contain whitespace or ':'"),
		}
	}
	for _, userID := range r.Invite {
		// TODO: We should put user ID parsing code into gomatrixserverlib and use that instead
		//       (see https://github.com/matrix-org/gomatrixserverlib/blob/3394e7c7003312043208aa73727d2256eea3d1f6/eventcontent.go#L347 )
		//       It should be a struct (with pointers into a single string to avoid copying) and
		//       we should update all refs to use UserID types rather than strings.
		// https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/types.py#L92
		if _, _, err := gomatrixserverlib.SplitID('@', userID); err != nil {
			return &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.BadJSON("user id must be in the form @localpart:domain"),
			}
		}
	}
	switch r.Preset {
	case presetPrivateChat, presetTrustedPrivateChat, presetPublicChat, "":
	default:
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("preset must be any of 'private_chat', 'trusted_private_chat', 'public_chat'"),
		}
	}

	// Validate creation_content fields defined in the spec by marshalling the
	// creation_content map into bytes and then unmarshalling the bytes into
	// common.CreateContent.

	creationContentBytes, err := json.Marshal(r.CreationContent)
	if err != nil {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("malformed creation_content"),
		}
	}

	var CreationContent gomatrixserverlib.CreateContent
	err = json.Unmarshal(creationContentBytes, &CreationContent)
	if err != nil {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("malformed creation_content"),
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
	req *http.Request, device *authtypes.Device,
	cfg config.Dendrite, producer *producers.RoomserverProducer,
	accountDB *accounts.Database, aliasAPI roomserverAPI.RoomserverAliasAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
	queryAPI roomserverAPI.RoomserverQueryAPI,
) util.JSONResponse {

	// Generate the room ID for our new room and reserve it from the roomserver.
	// Keep trying until we have a roomID which is unused, give up after 10 tries.
	var roomID string
	var try int
	for ; roomID == "" && try <= 10; try++ {
		checkRoomID := util.RandomString(16)
		checkRoomID = fmt.Sprintf("!%s:%s", checkRoomID, cfg.Matrix.ServerName)

		queryReq := roomserverAPI.QueryReserveRoomIDRequest{RoomID: checkRoomID}
		var queryResp roomserverAPI.QueryReserveRoomIDResponse

		err := queryAPI.QueryReserveRoomID(req.Context(), &queryReq, &queryResp)
		if err != nil {
			return httputil.LogThenError(req, err)
		}

		if queryResp.Success {
			roomID = checkRoomID
		}
	}

	if roomID == "" {
		return httputil.LogThenError(req, errors.New(
			"failed to determine a unused roomID for new room"))
	}

	return createRoom(req, device, cfg, roomID, producer, accountDB, aliasAPI, asAPI)
}

// createRoom implements /createRoom
// nolint: gocyclo
func createRoom(
	req *http.Request, device *authtypes.Device,
	cfg config.Dendrite, roomID string, producer *producers.RoomserverProducer,
	accountDB *accounts.Database, aliasAPI roomserverAPI.RoomserverAliasAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
) util.JSONResponse {
	logger := util.GetLogger(req.Context())
	userID := device.UserID
	var r createRoomRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}
	// TODO: apply rate-limit

	if resErr = r.Validate(); resErr != nil {
		return *resErr
	}

	evTime, err := httputil.ParseTSParam(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
	}

	// Clobber keys: creator, room_version

	if r.CreationContent == nil {
		r.CreationContent = make(map[string]interface{}, 2)
	}

	r.CreationContent["creator"] = userID
	r.CreationContent["room_version"] = "1" // TODO: We set this to 1 before we support Room versioning

	// TODO: visibility/presets/raw initial state
	// TODO: Create room alias association
	// Make sure this doesn't fall into an application service's namespace though!

	logger.WithFields(log.Fields{
		"userID": userID,
		"roomID": roomID,
	}).Info("Creating new room")

	profile, err := appserviceAPI.RetrieveUserProfile(req.Context(), userID, asAPI, accountDB)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	membershipContent := gomatrixserverlib.MemberContent{
		Membership:  gomatrixserverlib.Join,
		DisplayName: profile.DisplayName,
		AvatarURL:   profile.AvatarURL,
	}

	var joinRules, historyVisibility string
	switch r.Preset {
	case presetPrivateChat:
		joinRules = gomatrixserverlib.Invite
		historyVisibility = historyVisibilityShared
	case presetTrustedPrivateChat:
		joinRules = gomatrixserverlib.Invite
		historyVisibility = historyVisibilityShared
		// TODO If trusted_private_chat, all invitees are given the same power level as the room creator.
	case presetPublicChat:
		joinRules = gomatrixserverlib.Public
		historyVisibility = historyVisibilityShared
	default:
		// Default room rules, r.Preset was previously checked for valid values so
		// only a request with no preset should end up here.
		joinRules = gomatrixserverlib.Invite
		historyVisibility = historyVisibilityShared
	}

	var builtEvents []gomatrixserverlib.Event

	// send events into the room in order of:
	//  1- m.room.create
	//  2- room creator join member
	//  3- m.room.power_levels
	//  4- m.room.canonical_alias (opt) TODO
	//  5- m.room.join_rules
	//  6- m.room.history_visibility
	//  7- m.room.guest_access (opt)
	//  8- other initial state items
	//  9- m.room.name (opt)
	//  10- m.room.topic (opt)
	//  11- invite events (opt) - with is_direct flag if applicable TODO
	//  12- 3pid invite events (opt) TODO
	//  13- m.room.aliases event for HS (if alias specified) TODO
	// This differs from Synapse slightly. Synapse would vary the ordering of 3-7
	// depending on if those events were in "initial_state" or not. This made it
	// harder to reason about, hence sticking to a strict static ordering.
	// TODO: Synapse has txn/token ID on each event. Do we need to do this here?
	eventsToMake := []fledglingEvent{
		{"m.room.create", "", r.CreationContent},
		{"m.room.member", userID, membershipContent},
		{"m.room.power_levels", "", common.InitialPowerLevelsContent(userID)},
		// TODO: m.room.canonical_alias
		{"m.room.join_rules", "", gomatrixserverlib.JoinRuleContent{JoinRule: joinRules}},
		{"m.room.history_visibility", "", common.HistoryVisibilityContent{HistoryVisibility: historyVisibility}},
	}
	if r.GuestCanJoin {
		eventsToMake = append(eventsToMake, fledglingEvent{"m.room.guest_access", "", common.GuestAccessContent{GuestAccess: "can_join"}})
	}
	eventsToMake = append(eventsToMake, r.InitialState...)
	if r.Name != "" {
		eventsToMake = append(eventsToMake, fledglingEvent{"m.room.name", "", common.NameContent{Name: r.Name}})
	}
	if r.Topic != "" {
		eventsToMake = append(eventsToMake, fledglingEvent{"m.room.topic", "", common.TopicContent{Topic: r.Topic}})
	}
	// TODO: invite events
	// TODO: 3pid invite events
	// TODO: m.room.aliases

	authEvents := gomatrixserverlib.NewAuthEvents(nil)
	for i, e := range eventsToMake {
		depth := i + 1 // depth starts at 1

		builder := gomatrixserverlib.EventBuilder{
			Sender:   userID,
			RoomID:   roomID,
			Type:     e.Type,
			StateKey: &e.StateKey,
			Depth:    int64(depth),
		}
		err = builder.SetContent(e.Content)
		if err != nil {
			return httputil.LogThenError(req, err)
		}
		if i > 0 {
			builder.PrevEvents = []gomatrixserverlib.EventReference{builtEvents[i-1].EventReference()}
		}
		var ev *gomatrixserverlib.Event
		ev, err = buildEvent(&builder, &authEvents, cfg, evTime)
		if err != nil {
			return httputil.LogThenError(req, err)
		}

		if err = gomatrixserverlib.Allowed(*ev, &authEvents); err != nil {
			return httputil.LogThenError(req, err)
		}

		// Add the event to the list of auth events
		builtEvents = append(builtEvents, *ev)
		err = authEvents.AddEvent(ev)
		if err != nil {
			return httputil.LogThenError(req, err)
		}
	}

	// send events to the room server
	_, err = producer.SendEvents(req.Context(), builtEvents, cfg.Matrix.ServerName, nil)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	// TODO(#269): Reserve room alias while we create the room. This stops us
	// from creating the room but still failing due to the alias having already
	// been taken.
	var roomAlias string
	if r.RoomAliasName != "" {
		roomAlias = fmt.Sprintf("#%s:%s", r.RoomAliasName, cfg.Matrix.ServerName)

		aliasReq := roomserverAPI.SetRoomAliasRequest{
			Alias:  roomAlias,
			RoomID: roomID,
			UserID: userID,
		}

		var aliasResp roomserverAPI.SetRoomAliasResponse
		err = aliasAPI.SetRoomAlias(req.Context(), &aliasReq, &aliasResp)
		if err != nil {
			return httputil.LogThenError(req, err)
		}

		if aliasResp.AliasExists {
			return util.MessageResponse(400, "Alias already exists")
		}
	}

	response := createRoomResponse{
		RoomID:    roomID,
		RoomAlias: roomAlias,
	}

	return util.JSONResponse{
		Code: 200,
		JSON: response,
	}
}

// buildEvent fills out auth_events for the builder then builds the event
func buildEvent(
	builder *gomatrixserverlib.EventBuilder,
	provider gomatrixserverlib.AuthEventProvider,
	cfg config.Dendrite,
	evTime time.Time,
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
	eventID := fmt.Sprintf("$%s:%s", util.RandomString(16), cfg.Matrix.ServerName)
	event, err := builder.Build(eventID, evTime, cfg.Matrix.ServerName, cfg.Matrix.KeyID, cfg.Matrix.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("cannot build event %s : Builder failed to build. %s", builder.Type, err)
	}
	return &event, nil
}
