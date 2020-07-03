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

	"github.com/matrix-org/dendrite/appservice/types"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/dendrite/userapi/storage/devices"
	"github.com/matrix-org/gomatrixserverlib"
)

type UserInternalAPI struct {
	AccountDB  accounts.Database
	DeviceDB   devices.Database
	ServerName gomatrixserverlib.ServerName
	// AppServices is the list of all registered AS
	AppServices []config.ApplicationService
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
	return a.AccountDB.SaveAccountData(ctx, local, req.RoomID, req.DataType, req.AccountData)
}

func (a *UserInternalAPI) PerformAccountCreation(ctx context.Context, req *api.PerformAccountCreationRequest, res *api.PerformAccountCreationResponse) error {
	if req.AccountType == api.AccountTypeGuest {
		acc, err := a.AccountDB.CreateGuestAccount(ctx)
		if err != nil {
			return err
		}
		res.AccountCreated = true
		res.Account = acc
		return nil
	}
	acc, err := a.AccountDB.CreateAccount(ctx, req.Localpart, req.Password, req.AppServiceID)
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
		}
		return nil
	}

	if err = a.AccountDB.SetDisplayName(ctx, req.Localpart, req.Localpart); err != nil {
		return err
	}

	res.AccountCreated = true
	res.Account = acc
	return nil
}
func (a *UserInternalAPI) PerformDeviceCreation(ctx context.Context, req *api.PerformDeviceCreationRequest, res *api.PerformDeviceCreationResponse) error {
	dev, err := a.DeviceDB.CreateDevice(ctx, req.Localpart, req.DeviceID, req.AccessToken, req.DeviceDisplayName)
	if err != nil {
		return err
	}
	res.DeviceCreated = true
	res.Device = dev
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
	prof, err := a.AccountDB.GetProfileByLocalpart(ctx, local)
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

func (a *UserInternalAPI) QueryDevices(ctx context.Context, req *api.QueryDevicesRequest, res *api.QueryDevicesResponse) error {
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if domain != a.ServerName {
		return fmt.Errorf("cannot query devices of remote users: got %s want %s", domain, a.ServerName)
	}
	devs, err := a.DeviceDB.GetDevicesByLocalpart(ctx, local)
	if err != nil {
		return err
	}
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
		data, err = a.AccountDB.GetAccountDataByType(ctx, local, req.RoomID, req.DataType)
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
	global, rooms, err := a.AccountDB.GetAccountData(ctx, local)
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
		res.Device = appServiceDevice
		res.Err = err
		return nil
	}
	device, err := a.DeviceDB.GetDeviceByAccessToken(ctx, req.AccessToken)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
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
		AccessToken: token,
	}

	localpart, err := userutil.ParseUsernameParam(appServiceUserID, &a.ServerName)
	if err != nil {
		return nil, err
	}

	if localpart != "" { // AS is masquerading as another user
		// Verify that the user is registered
		account, err := a.AccountDB.GetAccountByLocalpart(ctx, localpart)
		// Verify that account exists & appServiceID matches
		if err == nil && account.AppServiceID == appService.ID {
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
