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

package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	userdb "github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/tokens"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/transactions"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// Unspecced server notice request
// https://github.com/matrix-org/synapse/blob/develop/docs/admin_api/server_notices.md
type sendServerNoticeRequest struct {
	UserID  string `json:"user_id,omitempty"`
	Content struct {
		MsgType string `json:"msgtype,omitempty"`
		Body    string `json:"body,omitempty"`
	} `json:"content,omitempty"`
	Type     string `json:"type,omitempty"`
	StateKey string `json:"state_key,omitempty"`
}

// SendServerNotice sends a message to a specific user. It can only be invoked by an admin.
func SendServerNotice(
	req *http.Request,
	cfgNotices *config.ServerNotices,
	cfgClient *config.ClientAPI,
	userAPI userapi.UserInternalAPI,
	rsAPI api.RoomserverInternalAPI,
	accountsDB userdb.Database,
	asAPI appserviceAPI.AppServiceQueryAPI,
	device *userapi.Device,
	senderDevice *userapi.Device,
	txnID *string,
	txnCache *transactions.Cache,
) util.JSONResponse {
	if device.AccountType != userapi.AccountTypeAdmin {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("This API can only be used by admin users."),
		}
	}

	if txnID != nil {
		// Try to fetch response from transactionsCache
		if res, ok := txnCache.FetchTransaction(device.AccessToken, *txnID); ok {
			return *res
		}
	}

	ctx := req.Context()
	var r sendServerNoticeRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	// check that all required fields are set
	if !r.valid() {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Invalid request"),
		}
	}

	// get rooms for specified user
	allUserRooms := []string{}
	userRooms := api.QueryRoomsForUserResponse{}
	if err := rsAPI.QueryRoomsForUser(ctx, &api.QueryRoomsForUserRequest{
		UserID:         r.UserID,
		WantMembership: "join",
	}, &userRooms); err != nil {
		return util.ErrorResponse(err)
	}
	allUserRooms = append(allUserRooms, userRooms.RoomIDs...)
	// get invites for specified user
	if err := rsAPI.QueryRoomsForUser(ctx, &api.QueryRoomsForUserRequest{
		UserID:         r.UserID,
		WantMembership: "invite",
	}, &userRooms); err != nil {
		return util.ErrorResponse(err)
	}
	allUserRooms = append(allUserRooms, userRooms.RoomIDs...)
	// get left rooms for specified user
	if err := rsAPI.QueryRoomsForUser(ctx, &api.QueryRoomsForUserRequest{
		UserID:         r.UserID,
		WantMembership: "leave",
	}, &userRooms); err != nil {
		return util.ErrorResponse(err)
	}
	allUserRooms = append(allUserRooms, userRooms.RoomIDs...)

	// get rooms of the sender
	senderUserID := fmt.Sprintf("@%s:%s", cfgNotices.LocalPart, cfgClient.Matrix.ServerName)
	senderRooms := api.QueryRoomsForUserResponse{}
	if err := rsAPI.QueryRoomsForUser(ctx, &api.QueryRoomsForUserRequest{
		UserID:         senderUserID,
		WantMembership: "join",
	}, &senderRooms); err != nil {
		return util.ErrorResponse(err)
	}

	// check if we have rooms in common
	commonRooms := []string{}
	for _, userRoomID := range allUserRooms {
		for _, senderRoomID := range senderRooms.RoomIDs {
			if userRoomID == senderRoomID {
				commonRooms = append(commonRooms, senderRoomID)
			}
		}
	}

	if len(commonRooms) > 1 {
		return util.ErrorResponse(fmt.Errorf("expected to find one room, but got %d", len(commonRooms)))
	}

	var (
		roomID      string
		roomVersion = gomatrixserverlib.RoomVersionV6
	)

	// create a new room for the user
	if len(commonRooms) == 0 {
		powerLevelContent := eventutil.InitialPowerLevelsContent(senderUserID)
		powerLevelContent.Users[r.UserID] = -10 // taken from Synapse
		pl, err := json.Marshal(powerLevelContent)
		if err != nil {
			return util.ErrorResponse(err)
		}
		createContent := map[string]interface{}{}
		createContent["m.federate"] = false
		cc, err := json.Marshal(createContent)
		if err != nil {
			return util.ErrorResponse(err)
		}
		crReq := createRoomRequest{
			Invite:                    []string{r.UserID},
			Name:                      cfgNotices.RoomName,
			Visibility:                "private",
			Preset:                    presetPrivateChat,
			CreationContent:           cc,
			GuestCanJoin:              false,
			RoomVersion:               roomVersion,
			PowerLevelContentOverride: pl,
		}

		roomRes := createRoom(ctx, crReq, senderDevice, cfgClient, accountsDB, rsAPI, asAPI, time.Now())

		switch data := roomRes.JSON.(type) {
		case createRoomResponse:
			roomID = data.RoomID

			// tag the room, so we can later check if the user tries to reject an invite
			serverAlertTag := gomatrix.TagContent{Tags: map[string]gomatrix.TagProperties{
				"m.server_notice": {
					Order: 1.0,
				},
			}}
			if err = saveTagData(req, r.UserID, roomID, userAPI, serverAlertTag); err != nil {
				util.GetLogger(ctx).WithError(err).Error("saveTagData failed")
				return jsonerror.InternalServerError()
			}

		default:
			// if we didn't get a createRoomResponse, we probably received an error, so return that.
			return roomRes
		}

	} else {
		// we've found a room in common, check the membership
		roomID = commonRooms[0]
		// re-invite the user
		res, err := sendInvite(ctx, accountsDB, senderDevice, roomID, r.UserID, "Server notice room", cfgClient, rsAPI, asAPI, time.Now())
		if err != nil {
			return res
		}
	}

	startedGeneratingEvent := time.Now()

	request := map[string]interface{}{
		"body":    r.Content.Body,
		"msgtype": r.Content.MsgType,
	}
	e, resErr := generateSendEvent(ctx, request, senderDevice, roomID, "m.room.message", nil, cfgClient, rsAPI, time.Now())
	if resErr != nil {
		logrus.Errorf("failed to send message: %+v", resErr)
		return *resErr
	}
	timeToGenerateEvent := time.Since(startedGeneratingEvent)

	var txnAndSessionID *api.TransactionID
	if txnID != nil {
		txnAndSessionID = &api.TransactionID{
			TransactionID: *txnID,
			SessionID:     device.SessionID,
		}
	}

	// pass the new event to the roomserver and receive the correct event ID
	// event ID in case of duplicate transaction is discarded
	startedSubmittingEvent := time.Now()
	if err := api.SendEvents(
		ctx, rsAPI,
		api.KindNew,
		[]*gomatrixserverlib.HeaderedEvent{
			e.Headered(roomVersion),
		},
		cfgClient.Matrix.ServerName,
		cfgClient.Matrix.ServerName,
		txnAndSessionID,
		false,
	); err != nil {
		util.GetLogger(ctx).WithError(err).Error("SendEvents failed")
		return jsonerror.InternalServerError()
	}
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"event_id":     e.EventID(),
		"room_id":      roomID,
		"room_version": roomVersion,
	}).Info("Sent event to roomserver")
	timeToSubmitEvent := time.Since(startedSubmittingEvent)

	res := util.JSONResponse{
		Code: http.StatusOK,
		JSON: sendEventResponse{e.EventID()},
	}
	// Add response to transactionsCache
	if txnID != nil {
		txnCache.AddTransaction(device.AccessToken, *txnID, &res)
	}

	// Take a note of how long it took to generate the event vs submit
	// it to the roomserver.
	sendEventDuration.With(prometheus.Labels{"action": "build"}).Observe(float64(timeToGenerateEvent.Milliseconds()))
	sendEventDuration.With(prometheus.Labels{"action": "submit"}).Observe(float64(timeToSubmitEvent.Milliseconds()))

	return res
}

func (r sendServerNoticeRequest) valid() (ok bool) {
	if r.UserID == "" {
		return false
	}
	if r.Content.MsgType == "" || r.Content.Body == "" {
		return false
	}
	return true
}

// getSenderDevice creates a user account to be used when sending server notices.
// It returns an userapi.Device, which is used for building the event
func getSenderDevice(
	ctx context.Context,
	userAPI userapi.UserInternalAPI,
	accountDB userdb.Database,
	cfg *config.ClientAPI,
) (*userapi.Device, error) {
	var accRes userapi.PerformAccountCreationResponse
	// create account if it doesn't exist
	err := userAPI.PerformAccountCreation(ctx, &userapi.PerformAccountCreationRequest{
		AccountType: userapi.AccountTypeUser,
		Localpart:   cfg.Matrix.ServerNotices.LocalPart,
		OnConflict:  userapi.ConflictUpdate,
	}, &accRes)
	if err != nil {
		return nil, err
	}

	// set the avatarurl for the user
	if err = accountDB.SetAvatarURL(ctx, cfg.Matrix.ServerNotices.LocalPart, cfg.Matrix.ServerNotices.AvatarURL); err != nil {
		util.GetLogger(ctx).WithError(err).Error("accountDB.SetAvatarURL failed")
		return nil, err
	}

	// Check if we got existing devices
	deviceRes := &userapi.QueryDevicesResponse{}
	err = userAPI.QueryDevices(ctx, &userapi.QueryDevicesRequest{
		UserID: accRes.Account.UserID,
	}, deviceRes)
	if err != nil {
		return nil, err
	}

	if len(deviceRes.Devices) > 0 {
		return &deviceRes.Devices[0], nil
	}

	// create an AccessToken
	token, err := tokens.GenerateLoginToken(tokens.TokenOptions{
		ServerPrivateKey: cfg.Matrix.PrivateKey.Seed(),
		ServerName:       string(cfg.Matrix.ServerName),
		UserID:           accRes.Account.UserID,
	})
	if err != nil {
		return nil, err
	}

	// create a new device, if we didn't find any
	var devRes userapi.PerformDeviceCreationResponse
	err = userAPI.PerformDeviceCreation(ctx, &userapi.PerformDeviceCreationRequest{
		Localpart:          cfg.Matrix.ServerNotices.LocalPart,
		DeviceDisplayName:  &cfg.Matrix.ServerNotices.LocalPart,
		AccessToken:        token,
		NoDeviceListUpdate: true,
	}, &devRes)

	if err != nil {
		return nil, err
	}
	return devRes.Device, nil
}
