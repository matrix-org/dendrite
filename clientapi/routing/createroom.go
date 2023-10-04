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

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	roomserverVersion "github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// https://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-createroom
type createRoomRequest struct {
	Invite                    []string                           `json:"invite"`
	Name                      string                             `json:"name"`
	Visibility                string                             `json:"visibility"`
	Topic                     string                             `json:"topic"`
	Preset                    string                             `json:"preset"`
	CreationContent           json.RawMessage                    `json:"creation_content"`
	InitialState              []gomatrixserverlib.FledglingEvent `json:"initial_state"`
	RoomAliasName             string                             `json:"room_alias_name"`
	RoomVersion               gomatrixserverlib.RoomVersion      `json:"room_version"`
	PowerLevelContentOverride json.RawMessage                    `json:"power_level_content_override"`
	IsDirect                  bool                               `json:"is_direct"`
	SenderID                  string                             `json:"sender_id"`
}

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
	case spec.PresetPrivateChat, spec.PresetTrustedPrivateChat, spec.PresetPublicChat, "":
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

// CreateRoomCryptoIDs implements /createRoom
func CreateRoomCryptoIDs(
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

	return makeCreateRoomEvents(req.Context(), createRequest, device, cfg, profileAPI, rsAPI, asAPI, evTime)
}

func makeCreateRoomEvents(
	ctx context.Context,
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

	// TODO: Check room ID doesn't clash with an existing one, and we
	//       probably shouldn't be using pseudo-random strings, maybe GUIDs?
	roomID, err := spec.NewRoomID(fmt.Sprintf("!%s:%s", util.RandomString(16), userID.Domain()))
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("invalid roomID")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// Clobber keys: creator, room_version

	roomVersion := rsAPI.DefaultRoomVersion()
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

	req := roomserverAPI.PerformCreateRoomRequest{
		InvitedUsers:              createRequest.Invite,
		RoomName:                  createRequest.Name,
		Visibility:                createRequest.Visibility,
		Topic:                     createRequest.Topic,
		StatePreset:               createRequest.Preset,
		CreationContent:           createRequest.CreationContent,
		InitialState:              createRequest.InitialState,
		RoomAliasName:             createRequest.RoomAliasName,
		RoomVersion:               roomVersion,
		PowerLevelContentOverride: createRequest.PowerLevelContentOverride,
		IsDirect:                  createRequest.IsDirect,

		UserDisplayName: userDisplayName,
		UserAvatarURL:   userAvatarURL,
		KeyID:           keyID,
		PrivateKey:      privateKey,
		EventTime:       evTime,

		SenderID: createRequest.SenderID,
	}

	createEvents, err := rsAPI.PerformCreateRoomCryptoIDs(ctx, *userID, *roomID, &req)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("MakeCreateRoomEvents failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{Err: err.Error()},
		}
	}

	response := createRoomCryptoIDsResponse{
		RoomID:  roomID.String(),
		Version: string(roomVersion),
		PDUs:    ToProtoEvents(ctx, createEvents, rsAPI),
	}

	return util.JSONResponse{
		Code: 200,
		JSON: response,
	}
}

type createRoomCryptoIDsResponse struct {
	RoomID  string            `json:"room_id"`
	Version string            `json:"room_version"`
	PDUs    []json.RawMessage `json:"pdus"`
}

func ToProtoEvents(ctx context.Context, events []gomatrixserverlib.PDU, rsAPI roomserverAPI.ClientRoomserverAPI) []json.RawMessage {
	result := make([]json.RawMessage, len(events))
	for i, event := range events {
		result[i] = json.RawMessage(event.JSON())
		//fmt.Printf("\nProcessing %s event (%s)\n", events[i].Type(), events[i].EventID())
		//var rawJson interface{}
		//json.Unmarshal(events[i].JSON(), &rawJson)
		//fmt.Printf("JSON: %+v\n", rawJson)
		//result[i] = gomatrixserverlib.ProtoEvent{
		//	SenderID:              string(events[i].SenderID()),
		//	RoomID:                events[i].RoomID().String(),
		//	Type:                  events[i].Type(),
		//	StateKey:              events[i].StateKey(),
		//	PrevEvents:            events[i].PrevEventIDs(),
		//	AuthEvents:            events[i].AuthEventIDs(),
		//	Redacts:               events[i].Redacts(),
		//	Depth:                 events[i].Depth(),
		//	Content:               events[i].Content(),
		//	Unsigned:              events[i].Unsigned(),
		//	Hashes:                events[i].Hashes(),
		//	OriginServerTimestamp: events[i].OriginServerTS(),
		//}

		//roomVersion, _ := rsAPI.QueryRoomVersionForRoom(ctx, events[i].RoomID().String())
		//verImpl, _ := gomatrixserverlib.GetRoomVersion(roomVersion)
		//eventJSON, err := json.Marshal(result[i])
		//if err != nil {
		//	util.GetLogger(ctx).WithError(err).Error("failed marshalling event")
		//	continue
		//}
		//pdu, err := verImpl.NewEventFromUntrustedJSON(eventJSON)
		//if err != nil {
		//	util.GetLogger(ctx).WithError(err).Error("failed making event from json")
		//	continue
		//}
		//fmt.Printf("\nProcessing %s event (%s) - PDU\n", result[i].Type, pdu.EventID())
		//fmt.Printf("  EventID: %v - %v\n", events[i].EventID(), pdu.EventID())
		//fmt.Printf("  SenderID: %s - %s\n", events[i].SenderID(), pdu.SenderID())
		//fmt.Printf("  RoomID: %s - %s\n", events[i].RoomID().String(), pdu.RoomID().String())
		//fmt.Printf("  Type: %s - %s\n", events[i].Type(), pdu.Type())
		//fmt.Printf("  StateKey: %s - %s\n", *events[i].StateKey(), *pdu.StateKey())
		//fmt.Printf("  PrevEvents: %v - %v\n", events[i].PrevEventIDs(), pdu.PrevEventIDs())
		//fmt.Printf("  AuthEvents: %v - %v\n", events[i].AuthEventIDs(), pdu.AuthEventIDs())
		//fmt.Printf("  Redacts: %s - %s\n", events[i].Redacts(), pdu.Redacts())
		//fmt.Printf("  Depth: %d - %d\n", events[i].Depth(), pdu.Depth())
		//fmt.Printf("  Content: %v - %v\n", events[i].Content(), pdu.Content())
		//fmt.Printf("  Unsigned: %v - %v\n", events[i].Unsigned(), pdu.Unsigned())
		//fmt.Printf("  Hashes: %v - %v\n", events[i].Hashes(), pdu.Hashes())
		//fmt.Printf("  OriginServerTS: %d - %d\n", events[i].OriginServerTS(), pdu.OriginServerTS())
		//json.Unmarshal(eventJSON, &rawJson)
		//fmt.Printf("JSON: %+v\n", rawJson)
	}
	return result
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

func createRoom(
	ctx context.Context,
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

	// TODO: Check room ID doesn't clash with an existing one, and we
	//       probably shouldn't be using pseudo-random strings, maybe GUIDs?
	roomID, err := spec.NewRoomID(fmt.Sprintf("!%s:%s", util.RandomString(16), userID.Domain()))
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("invalid roomID")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// Clobber keys: creator, room_version

	roomVersion := rsAPI.DefaultRoomVersion()
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

	req := roomserverAPI.PerformCreateRoomRequest{
		InvitedUsers:              createRequest.Invite,
		RoomName:                  createRequest.Name,
		Visibility:                createRequest.Visibility,
		Topic:                     createRequest.Topic,
		StatePreset:               createRequest.Preset,
		CreationContent:           createRequest.CreationContent,
		InitialState:              createRequest.InitialState,
		RoomAliasName:             createRequest.RoomAliasName,
		RoomVersion:               roomVersion,
		PowerLevelContentOverride: createRequest.PowerLevelContentOverride,
		IsDirect:                  createRequest.IsDirect,

		UserDisplayName: userDisplayName,
		UserAvatarURL:   userAvatarURL,
		KeyID:           keyID,
		PrivateKey:      privateKey,
		EventTime:       evTime,
	}

	roomAlias, createRes := rsAPI.PerformCreateRoom(ctx, *userID, *roomID, &req)
	if createRes != nil {
		return *createRes
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
