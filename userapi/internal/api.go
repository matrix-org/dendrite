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
	"fmt"

	"github.com/matrix-org/dendrite/appservice/types"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type UserInternalAPI struct {
	AccountDB  accounts.Database
	DeviceDB   devices.Database
	ServerName gomatrixserverlib.ServerName
	// AppServices is the list of all registered AS
	AppServices []config.ApplicationService
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
