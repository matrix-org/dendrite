// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	appserviceAPI "github.com/element-hq/dendrite/appservice/api"
	roomserverAPI "github.com/element-hq/dendrite/roomserver/api"
	roomserverVersion "github.com/element-hq/dendrite/roomserver/version"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/setup/config"
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
