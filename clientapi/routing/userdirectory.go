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
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrix"
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
	rsAPI api.ClientRoomserverAPI,
	provider userapi.QuerySearchProfilesAPI,
	searchString string,
	limit int,
	federation *gomatrixserverlib.FederationClient,
	localServerName gomatrixserverlib.ServerName,
) util.JSONResponse {
	if limit < 10 {
		limit = 10
	}

	results := map[string]authtypes.FullyQualifiedProfile{}
	response := &UserDirectoryResponse{
		Results: []authtypes.FullyQualifiedProfile{},
		Limited: false,
	}

	// Get users we share a room with
	knownUsersReq := &api.QueryKnownUsersRequest{
		UserID: device.UserID,
		Limit:  limit,
	}
	knownUsersRes := &api.QueryKnownUsersResponse{}
	if err := rsAPI.QueryKnownUsers(ctx, knownUsersReq, knownUsersRes); err != nil && err != sql.ErrNoRows {
		return util.ErrorResponse(fmt.Errorf("rsAPI.QueryKnownUsers: %w", err))
	}

knownUsersLoop:
	for _, profile := range knownUsersRes.Users {
		if len(results) == limit {
			response.Limited = true
			break
		}
		userID := profile.UserID
		// get the full profile of the local user
		localpart, serverName, _ := gomatrixserverlib.SplitID('@', userID)
		if serverName == localServerName {
			userReq := &userapi.QuerySearchProfilesRequest{
				SearchString: localpart,
				Limit:        limit,
			}
			userRes := &userapi.QuerySearchProfilesResponse{}
			if err := provider.QuerySearchProfiles(ctx, userReq, userRes); err != nil {
				return util.ErrorResponse(fmt.Errorf("userAPI.QuerySearchProfiles: %w", err))
			}
			for _, profile := range userRes.Profiles {
				if strings.Contains(userRes.Profiles[0].DisplayName, searchString) ||
					strings.Contains(userRes.Profiles[0].Localpart, searchString) {
					results[userID] = authtypes.FullyQualifiedProfile{
						UserID:      userID,
						DisplayName: profile.DisplayName,
						AvatarURL:   profile.AvatarURL,
					}
					if len(results) == limit {
						response.Limited = true
						break knownUsersLoop
					}
				}
			}
		} else {
			// If the username already contains the search string, don't bother hitting federation
			if strings.Contains(localpart, searchString) {
				results[userID] = authtypes.FullyQualifiedProfile{
					UserID:      userID,
					DisplayName: profile.DisplayName,
					AvatarURL:   profile.AvatarURL,
				}
				if len(results) == limit {
					response.Limited = true
					break knownUsersLoop
				}
			}
			// TODO: We should probably cache/store this
			profile, fedErr := federation.LookupProfile(ctx, serverName, userID, "")
			if fedErr != nil {
				if x, ok := fedErr.(gomatrix.HTTPError); ok {
					if x.Code == http.StatusNotFound {
						continue
					}
				}
			}
			if strings.Contains(profile.DisplayName, searchString) {
				results[userID] = authtypes.FullyQualifiedProfile{
					UserID:      userID,
					DisplayName: profile.DisplayName,
					AvatarURL:   profile.AvatarURL,
				}
				if len(results) == limit {
					response.Limited = true
					break knownUsersLoop
				}
			}
		}
	}

	for _, result := range results {
		response.Results = append(response.Results, result)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: response,
	}
}
