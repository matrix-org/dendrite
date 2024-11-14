// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	appserviceAPI "github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	fedsenderapi "github.com/element-hq/dendrite/federationapi/api"
	"github.com/element-hq/dendrite/internal/pushrules"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	clientapi "github.com/element-hq/dendrite/clientapi/api"
	"github.com/element-hq/dendrite/clientapi/userutil"
	"github.com/element-hq/dendrite/internal/eventutil"
	"github.com/element-hq/dendrite/internal/pushgateway"
	"github.com/element-hq/dendrite/internal/sqlutil"
	rsapi "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/config"
	synctypes "github.com/element-hq/dendrite/syncapi/types"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/element-hq/dendrite/userapi/producers"
	"github.com/element-hq/dendrite/userapi/storage"
	"github.com/element-hq/dendrite/userapi/storage/tables"
	userapiUtil "github.com/element-hq/dendrite/userapi/util"
)

type UserInternalAPI struct {
	DB                storage.UserDatabase
	KeyDatabase       storage.KeyDatabase
	SyncProducer      *producers.SyncAPI
	KeyChangeProducer *producers.KeyChange
	Config            *config.UserAPI

	DisableTLSValidation bool
	// AppServices is the list of all registered AS
	AppServices []config.ApplicationService
	RSAPI       rsapi.UserRoomserverAPI
	PgClient    pushgateway.Client
	FedClient   fedsenderapi.KeyserverFederationAPI
	Updater     *DeviceListUpdater
}

func (a *UserInternalAPI) PerformAdminCreateRegistrationToken(ctx context.Context, registrationToken *clientapi.RegistrationToken) (bool, error) {
	exists, err := a.DB.RegistrationTokenExists(ctx, *registrationToken.Token)
	if err != nil {
		return false, err
	}
	if exists {
		return false, fmt.Errorf("token: %s already exists", *registrationToken.Token)
	}
	_, err = a.DB.InsertRegistrationToken(ctx, registrationToken)
	if err != nil {
		return false, fmt.Errorf("Error creating token: %s"+err.Error(), *registrationToken.Token)
	}
	return true, nil
}

func (a *UserInternalAPI) PerformAdminListRegistrationTokens(ctx context.Context, returnAll bool, valid bool) ([]clientapi.RegistrationToken, error) {
	return a.DB.ListRegistrationTokens(ctx, returnAll, valid)
}

func (a *UserInternalAPI) PerformAdminGetRegistrationToken(ctx context.Context, tokenString string) (*clientapi.RegistrationToken, error) {
	return a.DB.GetRegistrationToken(ctx, tokenString)
}

func (a *UserInternalAPI) PerformAdminDeleteRegistrationToken(ctx context.Context, tokenString string) error {
	return a.DB.DeleteRegistrationToken(ctx, tokenString)
}

func (a *UserInternalAPI) PerformAdminUpdateRegistrationToken(ctx context.Context, tokenString string, newAttributes map[string]interface{}) (*clientapi.RegistrationToken, error) {
	return a.DB.UpdateRegistrationToken(ctx, tokenString, newAttributes)
}

func (a *UserInternalAPI) InputAccountData(ctx context.Context, req *api.InputAccountDataRequest, res *api.InputAccountDataResponse) error {
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return fmt.Errorf("cannot update account data of remote users (server name %s)", domain)
	}
	if req.DataType == "" {
		return fmt.Errorf("data type must not be empty")
	}
	if err := a.DB.SaveAccountData(ctx, local, domain, req.RoomID, req.DataType, req.AccountData); err != nil {
		util.GetLogger(ctx).WithError(err).Error("a.DB.SaveAccountData failed")
		return fmt.Errorf("failed to save account data: %w", err)
	}
	var ignoredUsers *synctypes.IgnoredUsers
	if req.DataType == "m.ignored_user_list" {
		ignoredUsers = &synctypes.IgnoredUsers{}
		_ = json.Unmarshal(req.AccountData, ignoredUsers)
	}
	if req.DataType == "m.fully_read" {
		if err := a.setFullyRead(ctx, req); err != nil {
			return err
		}
	}
	if err := a.SyncProducer.SendAccountData(req.UserID, eventutil.AccountData{
		RoomID:       req.RoomID,
		Type:         req.DataType,
		IgnoredUsers: ignoredUsers,
	}); err != nil {
		util.GetLogger(ctx).WithError(err).Error("a.SyncProducer.SendAccountData failed")
		return fmt.Errorf("failed to send account data to output: %w", err)
	}
	return nil
}

func (a *UserInternalAPI) setFullyRead(ctx context.Context, req *api.InputAccountDataRequest) error {
	var output eventutil.ReadMarkerJSON

	if err := json.Unmarshal(req.AccountData, &output); err != nil {
		return err
	}
	localpart, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		logrus.WithError(err).Error("UserInternalAPI.setFullyRead: SplitID failure")
		return nil
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return nil
	}

	deleted, err := a.DB.DeleteNotificationsUpTo(ctx, localpart, domain, req.RoomID, uint64(spec.AsTimestamp(time.Now())))
	if err != nil {
		logrus.WithError(err).Errorf("UserInternalAPI.setFullyRead: DeleteNotificationsUpTo failed")
		return err
	}

	if err = a.SyncProducer.GetAndSendNotificationData(ctx, req.UserID, req.RoomID); err != nil {
		logrus.WithError(err).Error("UserInternalAPI.setFullyRead: GetAndSendNotificationData failed")
		return err
	}

	// nothing changed, no need to notify the push gateway
	if !deleted {
		return nil
	}

	if err = userapiUtil.NotifyUserCountsAsync(ctx, a.PgClient, localpart, domain, a.DB); err != nil {
		logrus.WithError(err).Error("UserInternalAPI.setFullyRead: NotifyUserCounts failed")
		return err
	}
	return nil
}

func postRegisterJoinRooms(cfg *config.UserAPI, acc *api.Account, rsAPI rsapi.UserRoomserverAPI) {
	// POST register behaviour: check if the user is a normal user.
	// If the user is a normal user, add user to room specified in the configuration "auto_join_rooms".
	if acc.AccountType != api.AccountTypeAppService && acc.AppServiceID == "" {
		for room := range cfg.AutoJoinRooms {
			userID := userutil.MakeUserID(acc.Localpart, cfg.Matrix.ServerName)
			err := addUserToRoom(context.Background(), rsAPI, cfg.AutoJoinRooms[room], acc.Localpart, userID)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"user_id": userID,
					"room":    cfg.AutoJoinRooms[room],
				}).WithError(err).Errorf("user failed to auto-join room")
			}
		}
	}
}

// Add user to a room. This function currently working for auto_join_rooms config,
// which can add a newly registered user to a specified room.
func addUserToRoom(
	ctx context.Context,
	rsAPI rsapi.UserRoomserverAPI,
	roomID string,
	username string,
	userID string,
) error {
	addGroupContent := make(map[string]interface{})
	// This make sure the user's username can be displayed correctly.
	// Because the newly-registered user doesn't have an avatar, the avatar_url is not needed.
	addGroupContent["displayname"] = username
	joinReq := rsapi.PerformJoinRequest{
		RoomIDOrAlias: roomID,
		UserID:        userID,
		Content:       addGroupContent,
	}
	_, _, err := rsAPI.PerformJoin(ctx, &joinReq)
	return err
}

func (a *UserInternalAPI) PerformAccountCreation(ctx context.Context, req *api.PerformAccountCreationRequest, res *api.PerformAccountCreationResponse) error {
	serverName := req.ServerName
	if serverName == "" {
		serverName = a.Config.Matrix.ServerName
	}
	if !a.Config.Matrix.IsLocalServerName(serverName) {
		return fmt.Errorf("server name %s is not local", serverName)
	}
	acc, err := a.DB.CreateAccount(ctx, req.Localpart, serverName, req.Password, req.AppServiceID, req.AccountType)
	if err != nil {
		if errors.Is(err, sqlutil.ErrUserExists) { // This account already exists
			switch req.OnConflict {
			case api.ConflictUpdate:
				break
			case api.ConflictAbort:
				return &api.ErrorConflict{
					Message: err.Error(),
				}
			}
		}
		// account already exists
		res.AccountCreated = false
		res.Account = &api.Account{
			AppServiceID: req.AppServiceID,
			Localpart:    req.Localpart,
			ServerName:   serverName,
			UserID:       fmt.Sprintf("@%s:%s", req.Localpart, serverName),
			AccountType:  req.AccountType,
		}
		return nil
	}

	// Inform the SyncAPI about the newly created push_rules
	if err = a.SyncProducer.SendAccountData(acc.UserID, eventutil.AccountData{
		Type: "m.push_rules",
	}); err != nil {
		util.GetLogger(ctx).WithFields(logrus.Fields{
			"user_id": acc.UserID,
		}).WithError(err).Warn("failed to send account data to the SyncAPI")
	}

	if req.AccountType == api.AccountTypeGuest {
		res.AccountCreated = true
		res.Account = acc
		return nil
	}

	if _, _, err = a.DB.SetDisplayName(ctx, req.Localpart, serverName, req.Localpart); err != nil {
		return fmt.Errorf("a.DB.SetDisplayName: %w", err)
	}

	postRegisterJoinRooms(a.Config, acc, a.RSAPI)

	res.AccountCreated = true
	res.Account = acc
	return nil
}

func (a *UserInternalAPI) PerformPasswordUpdate(ctx context.Context, req *api.PerformPasswordUpdateRequest, res *api.PerformPasswordUpdateResponse) error {
	if !a.Config.Matrix.IsLocalServerName(req.ServerName) {
		return fmt.Errorf("server name %s is not local", req.ServerName)
	}
	if err := a.DB.SetPassword(ctx, req.Localpart, req.ServerName, req.Password); err != nil {
		return err
	}
	if req.LogoutDevices {
		if _, err := a.DB.RemoveAllDevices(context.Background(), req.Localpart, req.ServerName, ""); err != nil {
			return err
		}
	}
	res.PasswordUpdated = true
	return nil
}

func (a *UserInternalAPI) PerformDeviceCreation(ctx context.Context, req *api.PerformDeviceCreationRequest, res *api.PerformDeviceCreationResponse) error {
	serverName := req.ServerName
	if serverName == "" {
		serverName = a.Config.Matrix.ServerName
	}
	if !a.Config.Matrix.IsLocalServerName(serverName) {
		return fmt.Errorf("server name %s is not local", serverName)
	}
	// If a device ID was specified, check if it already exists and
	// avoid sending an empty device list update which would remove
	// existing device keys.
	isExisting := false
	if req.DeviceID != nil && *req.DeviceID != "" {
		existingDev, err := a.DB.GetDeviceByID(ctx, req.Localpart, req.ServerName, *req.DeviceID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		isExisting = existingDev.ID == *req.DeviceID
	}
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"localpart":    req.Localpart,
		"device_id":    req.DeviceID,
		"display_name": req.DeviceDisplayName,
	}).Info("PerformDeviceCreation")
	dev, err := a.DB.CreateDevice(ctx, req.Localpart, serverName, req.DeviceID, req.AccessToken, req.DeviceDisplayName, req.IPAddr, req.UserAgent)
	if err != nil {
		return err
	}
	res.DeviceCreated = true
	res.Device = dev
	if req.NoDeviceListUpdate || isExisting {
		return nil
	}
	// create empty device keys and upload them to trigger device list changes
	return a.deviceListUpdate(dev.UserID, []string{dev.ID}, req.FromRegistration)
}

func (a *UserInternalAPI) PerformDeviceDeletion(ctx context.Context, req *api.PerformDeviceDeletionRequest, res *api.PerformDeviceDeletionResponse) error {
	util.GetLogger(ctx).WithField("user_id", req.UserID).WithField("devices", req.DeviceIDs).Info("PerformDeviceDeletion")
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return fmt.Errorf("cannot PerformDeviceDeletion of remote users (server name %s)", domain)
	}
	deletedDeviceIDs := req.DeviceIDs
	if len(req.DeviceIDs) == 0 {
		var devices []api.Device
		devices, err = a.DB.RemoveAllDevices(ctx, local, domain, req.ExceptDeviceID)
		for _, d := range devices {
			deletedDeviceIDs = append(deletedDeviceIDs, d.ID)
		}
	} else {
		err = a.DB.RemoveDevices(ctx, local, domain, req.DeviceIDs)
	}
	if err != nil {
		return err
	}
	// Ask the keyserver to delete device keys and signatures for those devices
	deleteReq := &api.PerformDeleteKeysRequest{
		UserID: req.UserID,
	}
	for _, keyID := range req.DeviceIDs {
		deleteReq.KeyIDs = append(deleteReq.KeyIDs, gomatrixserverlib.KeyID(keyID))
	}
	deleteRes := &api.PerformDeleteKeysResponse{}
	if err := a.PerformDeleteKeys(ctx, deleteReq, deleteRes); err != nil {
		return err
	}
	if err := deleteRes.Error; err != nil {
		return fmt.Errorf("a.KeyAPI.PerformDeleteKeys: %w", err)
	}
	// create empty device keys and upload them to delete what was once there and trigger device list changes
	return a.deviceListUpdate(req.UserID, deletedDeviceIDs, false)
}

func (a *UserInternalAPI) deviceListUpdate(userID string, deviceIDs []string, fromRegistration bool) error {
	deviceKeys := make([]api.DeviceKeys, len(deviceIDs))
	for i, did := range deviceIDs {
		deviceKeys[i] = api.DeviceKeys{
			UserID:   userID,
			DeviceID: did,
			KeyJSON:  nil,
		}
	}

	var uploadRes api.PerformUploadKeysResponse
	if err := a.PerformUploadKeys(context.Background(), &api.PerformUploadKeysRequest{
		UserID:           userID,
		DeviceKeys:       deviceKeys,
		FromRegistration: fromRegistration,
	}, &uploadRes); err != nil {
		return err
	}
	if uploadRes.Error != nil {
		return fmt.Errorf("failed to delete device keys: %v", uploadRes.Error)
	}
	if len(uploadRes.KeyErrors) > 0 {
		return fmt.Errorf("failed to delete device keys, key errors: %+v", uploadRes.KeyErrors)
	}
	return nil
}

func (a *UserInternalAPI) PerformLastSeenUpdate(
	ctx context.Context,
	req *api.PerformLastSeenUpdateRequest,
	res *api.PerformLastSeenUpdateResponse,
) error {
	localpart, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return fmt.Errorf("server name %s is not local", domain)
	}
	if err := a.DB.UpdateDeviceLastSeen(ctx, localpart, domain, req.DeviceID, req.RemoteAddr, req.UserAgent); err != nil {
		return fmt.Errorf("a.DeviceDB.UpdateDeviceLastSeen: %w", err)
	}
	return nil
}

func (a *UserInternalAPI) PerformDeviceUpdate(ctx context.Context, req *api.PerformDeviceUpdateRequest, res *api.PerformDeviceUpdateResponse) error {
	localpart, domain, err := gomatrixserverlib.SplitID('@', req.RequestingUserID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return err
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return fmt.Errorf("server name %s is not local", domain)
	}
	dev, err := a.DB.GetDeviceByID(ctx, localpart, domain, req.DeviceID)
	if err == sql.ErrNoRows {
		res.DeviceExists = false
		return nil
	} else if err != nil {
		util.GetLogger(ctx).WithError(err).Error("deviceDB.GetDeviceByID failed")
		return err
	}
	res.DeviceExists = true

	err = a.DB.UpdateDevice(ctx, localpart, domain, req.DeviceID, req.DisplayName)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("deviceDB.UpdateDevice failed")
		return err
	}
	if req.DisplayName != nil && dev.DisplayName != *req.DisplayName {
		// display name has changed: update the device key
		var uploadRes api.PerformUploadKeysResponse
		if err := a.PerformUploadKeys(context.Background(), &api.PerformUploadKeysRequest{
			UserID: req.RequestingUserID,
			DeviceKeys: []api.DeviceKeys{
				{
					DeviceID:    dev.ID,
					DisplayName: *req.DisplayName,
					KeyJSON:     nil,
					UserID:      dev.UserID,
				},
			},
			OnlyDisplayNameUpdates: true,
		}, &uploadRes); err != nil {
			return err
		}
		if uploadRes.Error != nil {
			return fmt.Errorf("failed to update device key display name: %v", uploadRes.Error)
		}
		if len(uploadRes.KeyErrors) > 0 {
			return fmt.Errorf("failed to update device key display name, key errors: %+v", uploadRes.KeyErrors)
		}
	}
	return nil
}

var (
	ErrIsRemoteServer = errors.New("cannot query profile of remote users")
)

func (a *UserInternalAPI) QueryProfile(ctx context.Context, userID string) (*authtypes.Profile, error) {
	local, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return nil, ErrIsRemoteServer
	}
	prof, err := a.DB.GetProfileByLocalpart(ctx, local, domain)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, appserviceAPI.ErrProfileNotExists
		}
		return nil, err
	}
	return prof, nil
}

func (a *UserInternalAPI) QuerySearchProfiles(ctx context.Context, req *api.QuerySearchProfilesRequest, res *api.QuerySearchProfilesResponse) error {
	profiles, err := a.DB.SearchProfiles(ctx, req.SearchString, req.Limit)
	if err != nil {
		return err
	}
	res.Profiles = profiles
	return nil
}

func (a *UserInternalAPI) QueryDeviceInfos(ctx context.Context, req *api.QueryDeviceInfosRequest, res *api.QueryDeviceInfosResponse) error {
	devices, err := a.DB.GetDevicesByID(ctx, req.DeviceIDs)
	if err != nil {
		return err
	}
	res.DeviceInfo = make(map[string]struct {
		DisplayName string
		UserID      string
	})
	for _, d := range devices {
		res.DeviceInfo[d.ID] = struct {
			DisplayName string
			UserID      string
		}{
			DisplayName: d.DisplayName,
			UserID:      d.UserID,
		}
	}
	return nil
}

func (a *UserInternalAPI) QueryDevices(ctx context.Context, req *api.QueryDevicesRequest, res *api.QueryDevicesResponse) error {
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return fmt.Errorf("cannot query devices of remote users (server name %s)", domain)
	}
	devs, err := a.DB.GetDevicesByLocalpart(ctx, local, domain)
	if err != nil {
		return err
	}
	res.UserExists = true
	res.Devices = devs
	return nil
}

func (a *UserInternalAPI) QueryAccountData(ctx context.Context, req *api.QueryAccountDataRequest, res *api.QueryAccountDataResponse) error {
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return fmt.Errorf("cannot query account data of remote users (server name %s)", domain)
	}
	if req.DataType != "" {
		var data json.RawMessage
		data, err = a.DB.GetAccountDataByType(ctx, local, domain, req.RoomID, req.DataType)
		if err != nil {
			return err
		}
		res.RoomAccountData = make(map[string]map[string]json.RawMessage)
		res.GlobalAccountData = make(map[string]json.RawMessage)
		if data != nil {
			if req.RoomID != "" {
				if _, ok := res.RoomAccountData[req.RoomID]; !ok {
					res.RoomAccountData[req.RoomID] = make(map[string]json.RawMessage)
				}
				res.RoomAccountData[req.RoomID][req.DataType] = data
			} else {
				res.GlobalAccountData[req.DataType] = data
			}
		}
		return nil
	}
	global, rooms, err := a.DB.GetAccountData(ctx, local, domain)
	if err != nil {
		return err
	}
	res.RoomAccountData = rooms
	res.GlobalAccountData = global
	return nil
}

func (a *UserInternalAPI) QueryAccessToken(ctx context.Context, req *api.QueryAccessTokenRequest, res *api.QueryAccessTokenResponse) error {
	if req.AppServiceUserID != "" {
		appServiceDevice, err := a.queryAppServiceToken(ctx, req.AccessToken, req.AppServiceUserID)
		if err != nil || appServiceDevice != nil {
			if err != nil {
				res.Err = err.Error()
			}
			res.Device = appServiceDevice

			return nil
		}
		// If the provided token wasn't an as_token (both err and appServiceDevice are nil), continue with normal auth.
	}
	device, err := a.DB.GetDeviceByAccessToken(ctx, req.AccessToken)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	localPart, domain, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return err
	}
	if !a.Config.Matrix.IsLocalServerName(domain) {
		return nil
	}
	acc, err := a.DB.GetAccountByLocalpart(ctx, localPart, domain)
	if err != nil {
		return err
	}
	device.AccountType = acc.AccountType
	res.Device = device
	return nil
}

func (a *UserInternalAPI) QueryAccountByLocalpart(ctx context.Context, req *api.QueryAccountByLocalpartRequest, res *api.QueryAccountByLocalpartResponse) (err error) {
	res.Account, err = a.DB.GetAccountByLocalpart(ctx, req.Localpart, req.ServerName)
	return
}

// Return the appservice 'device' or nil if the token is not an appservice. Returns an error if there was a problem
// creating a 'device'.
func (a *UserInternalAPI) queryAppServiceToken(ctx context.Context, token, appServiceUserID string) (*api.Device, error) {
	// Search for app service with given access_token
	var appService *config.ApplicationService
	for _, as := range a.AppServices {
		if as.ASToken == token {
			appService = &as
			break
		}
	}
	if appService == nil {
		return nil, nil
	}

	// Create a dummy device for AS user
	dev := api.Device{
		// Use AS dummy device ID
		ID: "AS_Device",
		// AS dummy device has AS's token.
		AccessToken:  token,
		AppserviceID: appService.ID,
		AccountType:  api.AccountTypeAppService,
	}

	localpart, domain, err := userutil.ParseUsernameParam(appServiceUserID, a.Config.Matrix)
	if err != nil {
		return nil, err
	}

	if localpart != "" { // AS is masquerading as another user
		// Verify that the user is registered
		account, err := a.DB.GetAccountByLocalpart(ctx, localpart, domain)
		// Verify that the account exists and either appServiceID matches or
		// it belongs to the appservice user namespaces
		if err == nil && (account.AppServiceID == appService.ID || appService.IsInterestedInUserID(appServiceUserID)) {
			// Set the userID of dummy device
			dev.UserID = appServiceUserID
			return &dev, nil
		}
		return nil, &api.ErrorForbidden{Message: "appservice has not registered this user"}
	}

	// AS is not masquerading as any user, so use AS's sender_localpart
	dev.UserID = appService.SenderLocalpart
	return &dev, nil
}

// PerformAccountDeactivation deactivates the user's account, removing all ability for the user to login again.
func (a *UserInternalAPI) PerformAccountDeactivation(ctx context.Context, req *api.PerformAccountDeactivationRequest, res *api.PerformAccountDeactivationResponse) error {
	serverName := req.ServerName
	if serverName == "" {
		serverName = a.Config.Matrix.ServerName
	}
	if !a.Config.Matrix.IsLocalServerName(serverName) {
		return fmt.Errorf("server name %q not locally configured", serverName)
	}

	userID := fmt.Sprintf("@%s:%s", req.Localpart, serverName)
	_, err := a.RSAPI.PerformAdminEvacuateUser(ctx, userID)
	if err != nil {
		logrus.WithError(err).WithField("userID", userID).Errorf("Failed to evacuate user after account deactivation")
	}

	deviceReq := &api.PerformDeviceDeletionRequest{
		UserID: fmt.Sprintf("@%s:%s", req.Localpart, serverName),
	}
	deviceRes := &api.PerformDeviceDeletionResponse{}
	if err = a.PerformDeviceDeletion(ctx, deviceReq, deviceRes); err != nil {
		return err
	}

	pusherReq := &api.PerformPusherDeletionRequest{
		Localpart: req.Localpart,
	}
	if err = a.PerformPusherDeletion(ctx, pusherReq, &struct{}{}); err != nil {
		return err
	}

	err = a.DB.DeactivateAccount(ctx, req.Localpart, serverName)
	res.AccountDeactivated = err == nil
	return err
}

// PerformOpenIDTokenCreation creates a new token that a relying party uses to authenticate a user
func (a *UserInternalAPI) PerformOpenIDTokenCreation(ctx context.Context, req *api.PerformOpenIDTokenCreationRequest, res *api.PerformOpenIDTokenCreationResponse) error {
	token := util.RandomString(24)

	exp, err := a.DB.CreateOpenIDToken(ctx, token, req.UserID)

	res.Token = api.OpenIDToken{
		Token:       token,
		UserID:      req.UserID,
		ExpiresAtMS: exp,
	}

	return err
}

// QueryOpenIDToken validates that the OpenID token was issued for the user, the replying party uses this for validation
func (a *UserInternalAPI) QueryOpenIDToken(ctx context.Context, req *api.QueryOpenIDTokenRequest, res *api.QueryOpenIDTokenResponse) error {
	openIDTokenAttrs, err := a.DB.GetOpenIDTokenAttributes(ctx, req.Token)
	if err != nil {
		return err
	}

	res.Sub = openIDTokenAttrs.UserID
	res.ExpiresAtMS = openIDTokenAttrs.ExpiresAtMS

	return nil
}

func (a *UserInternalAPI) DeleteKeyBackup(ctx context.Context, userID, version string) (bool, error) {
	return a.DB.DeleteKeyBackup(ctx, userID, version)
}

func (a *UserInternalAPI) PerformKeyBackup(ctx context.Context, req *api.PerformKeyBackupRequest) (string, error) {
	// Create metadata
	return a.DB.CreateKeyBackup(ctx, req.UserID, req.Algorithm, req.AuthData)
}

func (a *UserInternalAPI) UpdateBackupKeyAuthData(ctx context.Context, req *api.PerformKeyBackupRequest) (*api.PerformKeyBackupResponse, error) {
	res := &api.PerformKeyBackupResponse{}
	// Update metadata
	if len(req.Keys.Rooms) == 0 {
		err := a.DB.UpdateKeyBackupAuthData(ctx, req.UserID, req.Version, req.AuthData)
		res.Exists = err == nil
		res.Version = req.Version
		if err != nil {
			return res, fmt.Errorf("failed to update backup: %w", err)
		}
		return res, nil
	}
	// Upload Keys for a specific version metadata
	return a.uploadBackupKeys(ctx, req)
}

func (a *UserInternalAPI) uploadBackupKeys(ctx context.Context, req *api.PerformKeyBackupRequest) (*api.PerformKeyBackupResponse, error) {
	res := &api.PerformKeyBackupResponse{}
	// you can only upload keys for the CURRENT version
	version, _, _, _, deleted, err := a.DB.GetKeyBackup(ctx, req.UserID, "")
	if err != nil {
		return res, fmt.Errorf("failed to query version: %w", err)
	}
	if deleted {
		return res, fmt.Errorf("backup was deleted")
	}
	if version != req.Version {
		return res, spec.WrongBackupVersionError(version)
	}
	res.Exists = true
	res.Version = version

	// map keys to a form we can upload more easily - the map ensures we have no duplicates.
	var uploads []api.InternalKeyBackupSession
	for roomID, data := range req.Keys.Rooms {
		for sessionID, sessionData := range data.Sessions {
			uploads = append(uploads, api.InternalKeyBackupSession{
				RoomID:           roomID,
				SessionID:        sessionID,
				KeyBackupSession: sessionData,
			})
		}
	}
	count, etag, err := a.DB.UpsertBackupKeys(ctx, version, req.UserID, uploads)
	if err != nil {
		return res, fmt.Errorf("failed to upsert keys: %w", err)
	}
	res.KeyCount = count
	res.KeyETag = etag
	return res, nil
}

func (a *UserInternalAPI) QueryKeyBackup(ctx context.Context, req *api.QueryKeyBackupRequest) (*api.QueryKeyBackupResponse, error) {
	res := &api.QueryKeyBackupResponse{}
	version, algorithm, authData, etag, deleted, err := a.DB.GetKeyBackup(ctx, req.UserID, req.Version)
	res.Version = version
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return res, nil
		}
		if errors.Is(err, strconv.ErrSyntax) {
			return res, nil
		}
		return res, fmt.Errorf("failed to query key backup: %s", err)
	}
	res.Algorithm = algorithm
	res.AuthData = authData
	res.ETag = etag
	res.Exists = !deleted

	if !req.ReturnKeys {
		res.Count, err = a.DB.CountBackupKeys(ctx, version, req.UserID)
		if err != nil {
			return res, fmt.Errorf("failed to count keys: %w", err)
		}
		return res, nil
	}

	result, err := a.DB.GetBackupKeys(ctx, version, req.UserID, req.KeysForRoomID, req.KeysForSessionID)
	if err != nil {
		return res, fmt.Errorf("failed to query keys: %s", err)
	}
	res.Keys = result
	return res, nil
}

func (a *UserInternalAPI) QueryNotifications(ctx context.Context, req *api.QueryNotificationsRequest, res *api.QueryNotificationsResponse) error {
	if req.Limit == 0 || req.Limit > 1000 {
		req.Limit = 1000
	}

	var fromID int64
	var err error
	if req.From != "" {
		fromID, err = strconv.ParseInt(req.From, 10, 64)
		if err != nil {
			return fmt.Errorf("QueryNotifications: parsing 'from': %w", err)
		}
	}
	var filter tables.NotificationFilter = tables.AllNotifications
	if req.Only == "highlight" {
		filter = tables.HighlightNotifications
	}
	notifs, lastID, err := a.DB.GetNotifications(ctx, req.Localpart, req.ServerName, fromID, req.Limit, filter)
	if err != nil {
		return err
	}
	if notifs == nil {
		// This ensures empty is JSON-encoded as [] instead of null.
		notifs = []*api.Notification{}
	}
	res.Notifications = notifs
	if lastID >= 0 {
		res.NextToken = strconv.FormatInt(lastID+1, 10)
	}
	return nil
}

func (a *UserInternalAPI) PerformPusherSet(ctx context.Context, req *api.PerformPusherSetRequest, res *struct{}) error {
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"localpart":    req.Localpart,
		"pushkey":      req.Pusher.PushKey,
		"display_name": req.Pusher.AppDisplayName,
	}).Info("PerformPusherCreation")
	if !req.Append {
		err := a.DB.RemovePushers(ctx, req.Pusher.AppID, req.Pusher.PushKey)
		if err != nil {
			return err
		}
	}
	if req.Pusher.Kind == "" {
		return a.DB.RemovePusher(ctx, req.Pusher.AppID, req.Pusher.PushKey, req.Localpart, req.ServerName)
	}
	if req.Pusher.PushKeyTS == 0 {
		req.Pusher.PushKeyTS = int64(time.Now().Unix())
	}
	return a.DB.UpsertPusher(ctx, req.Pusher, req.Localpart, req.ServerName)
}

func (a *UserInternalAPI) PerformPusherDeletion(ctx context.Context, req *api.PerformPusherDeletionRequest, res *struct{}) error {
	pushers, err := a.DB.GetPushers(ctx, req.Localpart, req.ServerName)
	if err != nil {
		return err
	}
	for i := range pushers {
		logrus.Warnf("pusher session: %d, req session: %d", pushers[i].SessionID, req.SessionID)
		if pushers[i].SessionID != req.SessionID {
			err := a.DB.RemovePusher(ctx, pushers[i].AppID, pushers[i].PushKey, req.Localpart, req.ServerName)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *UserInternalAPI) QueryPushers(ctx context.Context, req *api.QueryPushersRequest, res *api.QueryPushersResponse) error {
	var err error
	res.Pushers, err = a.DB.GetPushers(ctx, req.Localpart, req.ServerName)
	return err
}

func (a *UserInternalAPI) PerformPushRulesPut(
	ctx context.Context,
	userID string,
	ruleSets *pushrules.AccountRuleSets,
) error {
	bs, err := json.Marshal(ruleSets)
	if err != nil {
		return err
	}
	userReq := api.InputAccountDataRequest{
		UserID:      userID,
		DataType:    pushRulesAccountDataType,
		AccountData: json.RawMessage(bs),
	}
	var userRes api.InputAccountDataResponse // empty
	return a.InputAccountData(ctx, &userReq, &userRes)
}

func (a *UserInternalAPI) QueryPushRules(ctx context.Context, userID string) (*pushrules.AccountRuleSets, error) {
	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, fmt.Errorf("failed to split user ID %q for push rules", userID)
	}
	return a.DB.QueryPushRules(ctx, localpart, domain)
}

func (a *UserInternalAPI) SetAvatarURL(ctx context.Context, localpart string, serverName spec.ServerName, avatarURL string) (*authtypes.Profile, bool, error) {
	return a.DB.SetAvatarURL(ctx, localpart, serverName, avatarURL)
}

func (a *UserInternalAPI) QueryNumericLocalpart(ctx context.Context, req *api.QueryNumericLocalpartRequest, res *api.QueryNumericLocalpartResponse) error {
	id, err := a.DB.GetNewNumericLocalpart(ctx, req.ServerName)
	if err != nil {
		return err
	}
	res.ID = id
	return nil
}

func (a *UserInternalAPI) QueryAccountAvailability(ctx context.Context, req *api.QueryAccountAvailabilityRequest, res *api.QueryAccountAvailabilityResponse) error {
	var err error
	res.Available, err = a.DB.CheckAccountAvailability(ctx, req.Localpart, req.ServerName)
	return err
}

func (a *UserInternalAPI) QueryAccountByPassword(ctx context.Context, req *api.QueryAccountByPasswordRequest, res *api.QueryAccountByPasswordResponse) error {
	acc, err := a.DB.GetAccountByPassword(ctx, req.Localpart, req.ServerName, req.PlaintextPassword)
	switch err {
	case sql.ErrNoRows: // user does not exist
		return nil
	case bcrypt.ErrMismatchedHashAndPassword: // user exists, but password doesn't match
		return nil
	case bcrypt.ErrHashTooShort: // user exists, but probably a passwordless account
		return nil
	case nil:
		res.Exists = true
		res.Account = acc
		return nil
	}
	return err
}

func (a *UserInternalAPI) SetDisplayName(ctx context.Context, localpart string, serverName spec.ServerName, displayName string) (*authtypes.Profile, bool, error) {
	return a.DB.SetDisplayName(ctx, localpart, serverName, displayName)
}

func (a *UserInternalAPI) QueryLocalpartForThreePID(ctx context.Context, req *api.QueryLocalpartForThreePIDRequest, res *api.QueryLocalpartForThreePIDResponse) error {
	localpart, domain, err := a.DB.GetLocalpartForThreePID(ctx, req.ThreePID, req.Medium)
	if err != nil {
		return err
	}
	res.Localpart = localpart
	res.ServerName = domain
	return nil
}

func (a *UserInternalAPI) QueryThreePIDsForLocalpart(ctx context.Context, req *api.QueryThreePIDsForLocalpartRequest, res *api.QueryThreePIDsForLocalpartResponse) error {
	r, err := a.DB.GetThreePIDsForLocalpart(ctx, req.Localpart, req.ServerName)
	if err != nil {
		return err
	}
	res.ThreePIDs = r
	return nil
}

func (a *UserInternalAPI) PerformForgetThreePID(ctx context.Context, req *api.PerformForgetThreePIDRequest, res *struct{}) error {
	return a.DB.RemoveThreePIDAssociation(ctx, req.ThreePID, req.Medium)
}

func (a *UserInternalAPI) PerformSaveThreePIDAssociation(ctx context.Context, req *api.PerformSaveThreePIDAssociationRequest, res *struct{}) error {
	return a.DB.SaveThreePIDAssociation(ctx, req.ThreePID, req.Localpart, req.ServerName, req.Medium)
}

const pushRulesAccountDataType = "m.push_rules"
