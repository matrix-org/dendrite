// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/roomserver/api"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
	federation fclient.FederationClient,
	localServerName spec.ServerName,
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
			for _, p := range userRes.Profiles {
				if strings.Contains(p.DisplayName, searchString) ||
					strings.Contains(p.Localpart, searchString) {
					profile.DisplayName = p.DisplayName
					profile.AvatarURL = p.AvatarURL
					results[userID] = profile
					if len(results) == limit {
						response.Limited = true
						break knownUsersLoop
					}
				}
			}
		} else {
			// If the username already contains the search string, don't bother hitting federation.
			// This will result in missing avatars and displaynames, but saves the federation roundtrip.
			if strings.Contains(localpart, searchString) {
				results[userID] = profile
				if len(results) == limit {
					response.Limited = true
					break knownUsersLoop
				}
				continue
			}
			// TODO: We should probably cache/store this
			fedProfile, fedErr := federation.LookupProfile(ctx, localServerName, serverName, userID, "")
			if fedErr != nil {
				if x, ok := fedErr.(gomatrix.HTTPError); ok {
					if x.Code == http.StatusNotFound {
						continue
					}
				}
			}
			if strings.Contains(fedProfile.DisplayName, searchString) {
				profile.DisplayName = fedProfile.DisplayName
				profile.AvatarURL = fedProfile.AvatarURL
				results[userID] = profile
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
