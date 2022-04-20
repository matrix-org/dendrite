// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"

	"github.com/matrix-org/dendrite/appservice/types"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
)

type UserInternalAPI struct {
	DB           storage.Database
	SyncProducer *producers.SyncAPI

	DisableTLSValidation bool
	ServerName           gomatrixserverlib.ServerName
	// AppServices is the list of all registered AS
	AppServices []config.ApplicationService
	KeyAPI      keyapi.KeyInternalAPI
}

func (a *UserInternalAPI) InputAccountData(ctx context.Context, req *api.InputAccountDataRequest, res *api.InputAccountDataResponse) error {
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if domain != a.ServerName {
		return fmt.Errorf("cannot query profile of remote users: got %s want %s", domain, a.ServerName)
	}
	if req.DataType == "" {
		return fmt.Errorf("data type must not be empty")
	}
	return a.DB.SaveAccountData(ctx, local, req.RoomID, req.DataType, req.AccountData)
}

func (a *UserInternalAPI) PerformAccountCreation(ctx context.Context, req *api.PerformAccountCreationRequest, res *api.PerformAccountCreationResponse) error {
	acc, err := a.DB.CreateAccount(ctx, req.Localpart, req.Password, req.AppServiceID, req.AccountType)
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
			ServerName:   a.ServerName,
			UserID:       fmt.Sprintf("@%s:%s", req.Localpart, a.ServerName),
			AccountType:  req.AccountType,
		}
		return nil
	}

	if req.AccountType == api.AccountTypeGuest {
		res.AccountCreated = true
		res.Account = acc
		return nil
	}

	if err = a.DB.SetDisplayName(ctx, req.Localpart, req.Localpart); err != nil {
		return err
	}

	res.AccountCreated = true
	res.Account = acc
	return nil
}

func (a *UserInternalAPI) PerformPasswordUpdate(ctx context.Context, req *api.PerformPasswordUpdateRequest, res *api.PerformPasswordUpdateResponse) error {
	if err := a.DB.SetPassword(ctx, req.Localpart, req.Password); err != nil {
		return err
	}
	res.PasswordUpdated = true
	return nil
}

func (a *UserInternalAPI) PerformDeviceCreation(ctx context.Context, req *api.PerformDeviceCreationRequest, res *api.PerformDeviceCreationResponse) error {
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"localpart":    req.Localpart,
		"device_id":    req.DeviceID,
		"display_name": req.DeviceDisplayName,
	}).Info("PerformDeviceCreation")
	dev, err := a.DB.CreateDevice(ctx, req.Localpart, req.DeviceID, req.AccessToken, req.DeviceDisplayName, req.IPAddr, req.UserAgent)
	if err != nil {
		return err
	}
	res.DeviceCreated = true
	res.Device = dev
	if req.NoDeviceListUpdate {
		return nil
	}
	// create empty device keys and upload them to trigger device list changes
	return a.deviceListUpdate(dev.UserID, []string{dev.ID})
}

func (a *UserInternalAPI) PerformDeviceDeletion(ctx context.Context, req *api.PerformDeviceDeletionRequest, res *api.PerformDeviceDeletionResponse) error {
	util.GetLogger(ctx).WithField("user_id", req.UserID).WithField("devices", req.DeviceIDs).Info("PerformDeviceDeletion")
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if domain != a.ServerName {
		return fmt.Errorf("cannot PerformDeviceDeletion of remote users: got %s want %s", domain, a.ServerName)
	}
	deletedDeviceIDs := req.DeviceIDs
	if len(req.DeviceIDs) == 0 {
		var devices []api.Device
		devices, err = a.DB.RemoveAllDevices(ctx, local, req.ExceptDeviceID)
		for _, d := range devices {
			deletedDeviceIDs = append(deletedDeviceIDs, d.ID)
		}
	} else {
		err = a.DB.RemoveDevices(ctx, local, req.DeviceIDs)
	}
	if err != nil {
		return err
	}
	// Ask the keyserver to delete device keys and signatures for those devices
	deleteReq := &keyapi.PerformDeleteKeysRequest{
		UserID: req.UserID,
	}
	for _, keyID := range req.DeviceIDs {
		deleteReq.KeyIDs = append(deleteReq.KeyIDs, gomatrixserverlib.KeyID(keyID))
	}
	deleteRes := &keyapi.PerformDeleteKeysResponse{}
	a.KeyAPI.PerformDeleteKeys(ctx, deleteReq, deleteRes)
	if err := deleteRes.Error; err != nil {
		return fmt.Errorf("a.KeyAPI.PerformDeleteKeys: %w", err)
	}
	// create empty device keys and upload them to delete what was once there and trigger device list changes
	return a.deviceListUpdate(req.UserID, deletedDeviceIDs)
}

func (a *UserInternalAPI) deviceListUpdate(userID string, deviceIDs []string) error {
	deviceKeys := make([]keyapi.DeviceKeys, len(deviceIDs))
	for i, did := range deviceIDs {
		deviceKeys[i] = keyapi.DeviceKeys{
			UserID:   userID,
			DeviceID: did,
			KeyJSON:  nil,
		}
	}

	var uploadRes keyapi.PerformUploadKeysResponse
	a.KeyAPI.PerformUploadKeys(context.Background(), &keyapi.PerformUploadKeysRequest{
		UserID:     userID,
		DeviceKeys: deviceKeys,
	}, &uploadRes)
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
	localpart, _, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
	}
	if err := a.DB.UpdateDeviceLastSeen(ctx, localpart, req.DeviceID, req.RemoteAddr); err != nil {
		return fmt.Errorf("a.DeviceDB.UpdateDeviceLastSeen: %w", err)
	}
	return nil
}

func (a *UserInternalAPI) PerformDeviceUpdate(ctx context.Context, req *api.PerformDeviceUpdateRequest, res *api.PerformDeviceUpdateResponse) error {
	localpart, _, err := gomatrixserverlib.SplitID('@', req.RequestingUserID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return err
	}
	dev, err := a.DB.GetDeviceByID(ctx, localpart, req.DeviceID)
	if err == sql.ErrNoRows {
		res.DeviceExists = false
		return nil
	} else if err != nil {
		util.GetLogger(ctx).WithError(err).Error("deviceDB.GetDeviceByID failed")
		return err
	}
	res.DeviceExists = true

	if dev.UserID != req.RequestingUserID {
		res.Forbidden = true
		return nil
	}

	err = a.DB.UpdateDevice(ctx, localpart, req.DeviceID, req.DisplayName)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("deviceDB.UpdateDevice failed")
		return err
	}
	if req.DisplayName != nil && dev.DisplayName != *req.DisplayName {
		// display name has changed: update the device key
		var uploadRes keyapi.PerformUploadKeysResponse
		a.KeyAPI.PerformUploadKeys(context.Background(), &keyapi.PerformUploadKeysRequest{
			UserID: req.RequestingUserID,
			DeviceKeys: []keyapi.DeviceKeys{
				{
					DeviceID:    dev.ID,
					DisplayName: *req.DisplayName,
					KeyJSON:     nil,
					UserID:      dev.UserID,
				},
			},
			OnlyDisplayNameUpdates: true,
		}, &uploadRes)
		if uploadRes.Error != nil {
			return fmt.Errorf("failed to update device key display name: %v", uploadRes.Error)
		}
		if len(uploadRes.KeyErrors) > 0 {
			return fmt.Errorf("failed to update device key display name, key errors: %+v", uploadRes.KeyErrors)
		}
	}
	return nil
}

func (a *UserInternalAPI) QueryProfile(ctx context.Context, req *api.QueryProfileRequest, res *api.QueryProfileResponse) error {
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if domain != a.ServerName {
		return fmt.Errorf("cannot query profile of remote users: got %s want %s", domain, a.ServerName)
	}
	prof, err := a.DB.GetProfileByLocalpart(ctx, local)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	res.UserExists = true
	res.AvatarURL = prof.AvatarURL
	res.DisplayName = prof.DisplayName
	return nil
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
	if domain != a.ServerName {
		return fmt.Errorf("cannot query devices of remote users: got %s want %s", domain, a.ServerName)
	}
	devs, err := a.DB.GetDevicesByLocalpart(ctx, local)
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
	if domain != a.ServerName {
		return fmt.Errorf("cannot query account data of remote users: got %s want %s", domain, a.ServerName)
	}
	if req.DataType != "" {
		var data json.RawMessage
		data, err = a.DB.GetAccountDataByType(ctx, local, req.RoomID, req.DataType)
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
	global, rooms, err := a.DB.GetAccountData(ctx, local)
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
		if err != nil {
			res.Err = err.Error()
		}
		res.Device = appServiceDevice

		return nil
	}
	device, err := a.DB.GetDeviceByAccessToken(ctx, req.AccessToken)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	localPart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return err
	}
	acc, err := a.DB.GetAccountByLocalpart(ctx, localPart)
	if err != nil {
		return err
	}
	device.AccountType = acc.AccountType
	res.Device = device
	return nil
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
		ID: types.AppServiceDeviceID,
		// AS dummy device has AS's token.
		AccessToken:  token,
		AppserviceID: appService.ID,
		AccountType:  api.AccountTypeAppService,
	}

	localpart, err := userutil.ParseUsernameParam(appServiceUserID, &a.ServerName)
	if err != nil {
		return nil, err
	}

	if localpart != "" { // AS is masquerading as another user
		// Verify that the user is registered
		account, err := a.DB.GetAccountByLocalpart(ctx, localpart)
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
	err := a.DB.DeactivateAccount(ctx, req.Localpart)
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

func (a *UserInternalAPI) PerformKeyBackup(ctx context.Context, req *api.PerformKeyBackupRequest, res *api.PerformKeyBackupResponse) error {
	// Delete metadata
	if req.DeleteBackup {
		if req.Version == "" {
			res.BadInput = true
			res.Error = "must specify a version to delete"
			if res.Error != "" {
				return fmt.Errorf(res.Error)
			}
			return nil
		}
		exists, err := a.DB.DeleteKeyBackup(ctx, req.UserID, req.Version)
		if err != nil {
			res.Error = fmt.Sprintf("failed to delete backup: %s", err)
		}
		res.Exists = exists
		res.Version = req.Version
		if res.Error != "" {
			return fmt.Errorf(res.Error)
		}
		return nil
	}
	// Create metadata
	if req.Version == "" {
		version, err := a.DB.CreateKeyBackup(ctx, req.UserID, req.Algorithm, req.AuthData)
		if err != nil {
			res.Error = fmt.Sprintf("failed to create backup: %s", err)
		}
		res.Exists = err == nil
		res.Version = version
		if res.Error != "" {
			return fmt.Errorf(res.Error)
		}
		return nil
	}
	// Update metadata
	if len(req.Keys.Rooms) == 0 {
		err := a.DB.UpdateKeyBackupAuthData(ctx, req.UserID, req.Version, req.AuthData)
		if err != nil {
			res.Error = fmt.Sprintf("failed to update backup: %s", err)
		}
		res.Exists = err == nil
		res.Version = req.Version
		if res.Error != "" {
			return fmt.Errorf(res.Error)
		}
		return nil
	}
	// Upload Keys for a specific version metadata
	a.uploadBackupKeys(ctx, req, res)
	if res.Error != "" {
		return fmt.Errorf(res.Error)
	}
	return nil
}

func (a *UserInternalAPI) uploadBackupKeys(ctx context.Context, req *api.PerformKeyBackupRequest, res *api.PerformKeyBackupResponse) {
	// you can only upload keys for the CURRENT version
	version, _, _, _, deleted, err := a.DB.GetKeyBackup(ctx, req.UserID, "")
	if err != nil {
		res.Error = fmt.Sprintf("failed to query version: %s", err)
		return
	}
	if deleted {
		res.Error = "backup was deleted"
		return
	}
	if version != req.Version {
		res.BadInput = true
		res.Error = fmt.Sprintf("%s isn't the current version, %s is.", req.Version, version)
		return
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
		res.Error = fmt.Sprintf("failed to upsert keys: %s", err)
		return
	}
	res.KeyCount = count
	res.KeyETag = etag
}

func (a *UserInternalAPI) QueryKeyBackup(ctx context.Context, req *api.QueryKeyBackupRequest, res *api.QueryKeyBackupResponse) {
	version, algorithm, authData, etag, deleted, err := a.DB.GetKeyBackup(ctx, req.UserID, req.Version)
	res.Version = version
	if err != nil {
		if err == sql.ErrNoRows {
			res.Exists = false
			return
		}
		res.Error = fmt.Sprintf("failed to query key backup: %s", err)
		return
	}
	res.Algorithm = algorithm
	res.AuthData = authData
	res.ETag = etag
	res.Exists = !deleted

	if !req.ReturnKeys {
		res.Count, err = a.DB.CountBackupKeys(ctx, version, req.UserID)
		if err != nil {
			res.Error = fmt.Sprintf("failed to count keys: %s", err)
		}
		return
	}

	result, err := a.DB.GetBackupKeys(ctx, version, req.UserID, req.KeysForRoomID, req.KeysForSessionID)
	if err != nil {
		res.Error = fmt.Sprintf("failed to query keys: %s", err)
		return
	}
	res.Keys = result
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
	notifs, lastID, err := a.DB.GetNotifications(ctx, req.Localpart, fromID, req.Limit, filter)
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
		return a.DB.RemovePusher(ctx, req.Pusher.AppID, req.Pusher.PushKey, req.Localpart)
	}
	if req.Pusher.PushKeyTS == 0 {
		req.Pusher.PushKeyTS = int64(time.Now().Unix())
	}
	return a.DB.UpsertPusher(ctx, req.Pusher, req.Localpart)
}

func (a *UserInternalAPI) PerformPusherDeletion(ctx context.Context, req *api.PerformPusherDeletionRequest, res *struct{}) error {
	pushers, err := a.DB.GetPushers(ctx, req.Localpart)
	if err != nil {
		return err
	}
	for i := range pushers {
		logrus.Warnf("pusher session: %d, req session: %d", pushers[i].SessionID, req.SessionID)
		if pushers[i].SessionID != req.SessionID {
			err := a.DB.RemovePusher(ctx, pushers[i].AppID, pushers[i].PushKey, req.Localpart)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *UserInternalAPI) QueryPushers(ctx context.Context, req *api.QueryPushersRequest, res *api.QueryPushersResponse) error {
	var err error
	res.Pushers, err = a.DB.GetPushers(ctx, req.Localpart)
	return err
}

func (a *UserInternalAPI) PerformPushRulesPut(
	ctx context.Context,
	req *api.PerformPushRulesPutRequest,
	_ *struct{},
) error {
	bs, err := json.Marshal(&req.RuleSets)
	if err != nil {
		return err
	}
	userReq := api.InputAccountDataRequest{
		UserID:      req.UserID,
		DataType:    pushRulesAccountDataType,
		AccountData: json.RawMessage(bs),
	}
	var userRes api.InputAccountDataResponse // empty
	if err := a.InputAccountData(ctx, &userReq, &userRes); err != nil {
		return err
	}

	if err := a.SyncProducer.SendAccountData(req.UserID, "" /* roomID */, pushRulesAccountDataType); err != nil {
		util.GetLogger(ctx).WithError(err).Errorf("syncProducer.SendData failed")
	}

	return nil
}

func (a *UserInternalAPI) QueryPushRules(ctx context.Context, req *api.QueryPushRulesRequest, res *api.QueryPushRulesResponse) error {
	userReq := api.QueryAccountDataRequest{
		UserID:   req.UserID,
		DataType: pushRulesAccountDataType,
	}
	var userRes api.QueryAccountDataResponse
	if err := a.QueryAccountData(ctx, &userReq, &userRes); err != nil {
		return err
	}
	bs, ok := userRes.GlobalAccountData[pushRulesAccountDataType]
	if ok {
		// Legacy Dendrite users will have completely empty push rules, so we should
		// detect that situation and set some defaults.
		var rules struct {
			G struct {
				Content   []json.RawMessage `json:"content"`
				Override  []json.RawMessage `json:"override"`
				Room      []json.RawMessage `json:"room"`
				Sender    []json.RawMessage `json:"sender"`
				Underride []json.RawMessage `json:"underride"`
			} `json:"global"`
		}
		if err := json.Unmarshal([]byte(bs), &rules); err == nil {
			count := len(rules.G.Content) + len(rules.G.Override) +
				len(rules.G.Room) + len(rules.G.Sender) + len(rules.G.Underride)
			ok = count > 0
		}
	}
	if !ok {
		// If we didn't find any default push rules then we should just generate some
		// fresh ones.
		localpart, _, err := gomatrixserverlib.SplitID('@', req.UserID)
		if err != nil {
			return fmt.Errorf("failed to split user ID %q for push rules", req.UserID)
		}
		pushRuleSets := pushrules.DefaultAccountRuleSets(localpart, a.ServerName)
		prbs, err := json.Marshal(pushRuleSets)
		if err != nil {
			return fmt.Errorf("failed to marshal default push rules: %w", err)
		}
		if err := a.DB.SaveAccountData(ctx, localpart, "", pushRulesAccountDataType, json.RawMessage(prbs)); err != nil {
			return fmt.Errorf("failed to save default push rules: %w", err)
		}
		res.RuleSets = pushRuleSets
		return nil
	}
	var data pushrules.AccountRuleSets
	if err := json.Unmarshal([]byte(bs), &data); err != nil {
		util.GetLogger(ctx).WithError(err).Error("json.Unmarshal of push rules failed")
		return err
	}
	res.RuleSets = &data
	return nil
}

func (a *UserInternalAPI) SetAvatarURL(ctx context.Context, req *api.PerformSetAvatarURLRequest, res *api.PerformSetAvatarURLResponse) error {
	return a.DB.SetAvatarURL(ctx, req.Localpart, req.AvatarURL)
}

func (a *UserInternalAPI) QueryNumericLocalpart(ctx context.Context, res *api.QueryNumericLocalpartResponse) error {
	id, err := a.DB.GetNewNumericLocalpart(ctx)
	if err != nil {
		return err
	}
	res.ID = id
	return nil
}

func (a *UserInternalAPI) QueryAccountAvailability(ctx context.Context, req *api.QueryAccountAvailabilityRequest, res *api.QueryAccountAvailabilityResponse) error {
	var err error
	res.Available, err = a.DB.CheckAccountAvailability(ctx, req.Localpart)
	return err
}

func (a *UserInternalAPI) QueryAccountByPassword(ctx context.Context, req *api.QueryAccountByPasswordRequest, res *api.QueryAccountByPasswordResponse) error {
	acc, err := a.DB.GetAccountByPassword(ctx, req.Localpart, req.PlaintextPassword)
	switch err {
	case sql.ErrNoRows: // user does not exist
		return nil
	case bcrypt.ErrMismatchedHashAndPassword: // user exists, but password doesn't match
		return nil
	default:
		res.Exists = true
		res.Account = acc
		return nil
	}
}

func (a *UserInternalAPI) SetDisplayName(ctx context.Context, req *api.PerformUpdateDisplayNameRequest, _ *struct{}) error {
	return a.DB.SetDisplayName(ctx, req.Localpart, req.DisplayName)
}

func (a *UserInternalAPI) QueryLocalpartForThreePID(ctx context.Context, req *api.QueryLocalpartForThreePIDRequest, res *api.QueryLocalpartForThreePIDResponse) error {
	localpart, err := a.DB.GetLocalpartForThreePID(ctx, req.ThreePID, req.Medium)
	if err != nil {
		return err
	}
	res.Localpart = localpart
	return nil
}

func (a *UserInternalAPI) QueryThreePIDsForLocalpart(ctx context.Context, req *api.QueryThreePIDsForLocalpartRequest, res *api.QueryThreePIDsForLocalpartResponse) error {
	r, err := a.DB.GetThreePIDsForLocalpart(ctx, req.Localpart)
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
	return a.DB.SaveThreePIDAssociation(ctx, req.ThreePID, req.Localpart, req.Medium)
}

const pushRulesAccountDataType = "m.push_rules"
