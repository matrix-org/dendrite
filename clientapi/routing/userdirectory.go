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

package routing

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type UserDirectoryResponse struct {
	Results []authtypes.FullyQualifiedProfile `json:"results"`
	Limited bool                              `json:"limited"`
}

func SearchUserDirectory(
	ctx context.Context,
	device *userapi.Device,
	userAPI userapi.UserInternalAPI,
	rsAPI api.RoomserverInternalAPI,
	provider userapi.UserDirectoryProvider,
	serverName gomatrixserverlib.ServerName,
	searchString string,
	limit int,
) *util.JSONResponse {
	if limit < 10 {
		limit = 10
	}

	results := map[string]authtypes.FullyQualifiedProfile{}
	response := &UserDirectoryResponse{
		Results: []authtypes.FullyQualifiedProfile{},
		Limited: false,
	}

	// First start searching local users.
	userReq := &userapi.QuerySearchProfilesRequest{
		SearchString: searchString,
		Limit:        limit,
	}
	userRes := &userapi.QuerySearchProfilesResponse{}
	if err := provider.QuerySearchProfiles(ctx, userReq, userRes); err != nil {
		errRes := util.ErrorResponse(fmt.Errorf("userAPI.QuerySearchProfiles: %w", err))
		return &errRes
	}

	for _, user := range userRes.Profiles {
		if len(results) == limit {
			response.Limited = true
			break
		}

		var userID string
		if user.ServerName != "" {
			userID = fmt.Sprintf("@%s:%s", user.Localpart, user.ServerName)
		} else {
			userID = fmt.Sprintf("@%s:%s", user.Localpart, serverName)
		}
		if _, ok := results[userID]; !ok {
			results[userID] = authtypes.FullyQualifiedProfile{
				UserID:      userID,
				DisplayName: user.DisplayName,
				AvatarURL:   user.AvatarURL,
			}
		}
	}

	// Then, if we have enough room left in the response,
	// start searching for known users from joined rooms.

	if len(results) <= limit {
		stateReq := &api.QueryKnownUsersRequest{
			UserID:       device.UserID,
			SearchString: searchString,
			Limit:        limit - len(results),
		}
		stateRes := &api.QueryKnownUsersResponse{}
		if err := rsAPI.QueryKnownUsers(ctx, stateReq, stateRes); err != nil && err != sql.ErrNoRows {
			errRes := util.ErrorResponse(fmt.Errorf("rsAPI.QueryKnownUsers: %w", err))
			return &errRes
		}

		for _, user := range stateRes.Users {
			if len(results) == limit {
				response.Limited = true
				break
			}

			if _, ok := results[user.UserID]; !ok {
				results[user.UserID] = user
			}
		}
	}

	for _, result := range results {
		response.Results = append(response.Results, result)
	}

	return &util.JSONResponse{
		Code: 200,
		JSON: response,
	}
}
