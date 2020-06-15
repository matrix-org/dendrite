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

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type UserInternalAPI struct {
	AccountDB  accounts.Database
	DeviceDB   devices.Database
	ServerName gomatrixserverlib.ServerName
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
