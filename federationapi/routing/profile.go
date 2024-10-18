// Copyright 2017-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"errors"
	"fmt"
	"net/http"

	appserviceAPI "github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/internal/eventutil"
	"github.com/element-hq/dendrite/setup/config"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// GetProfile implements GET /_matrix/federation/v1/query/profile
func GetProfile(
	httpReq *http.Request,
	userAPI userapi.FederationUserAPI,
	cfg *config.FederationAPI,
) util.JSONResponse {
	userID, field := httpReq.FormValue("user_id"), httpReq.FormValue("field")

	// httpReq.FormValue will return an empty string if value is not found
	if userID == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MissingParam("The request body did not contain required argument 'user_id'."),
		}
	}

	_, domain, err := cfg.Matrix.SplitLocalID('@', userID)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(fmt.Sprintf("Domain %q does not match this server", domain)),
		}
	}

	profile, err := userAPI.QueryProfile(httpReq.Context(), userID)
	if err != nil {
		if errors.Is(err, appserviceAPI.ErrProfileNotExists) {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: spec.NotFound("The user does not exist or does not have a profile."),
			}
		}
		util.GetLogger(httpReq.Context()).WithError(err).Error("userAPI.QueryProfile failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	var res interface{}
	code := http.StatusOK

	if field != "" {
		switch field {
		case "displayname":
			res = eventutil.UserProfile{
				DisplayName: profile.DisplayName,
			}
		case "avatar_url":
			res = eventutil.UserProfile{
				AvatarURL: profile.AvatarURL,
			}
		default:
			code = http.StatusBadRequest
			res = spec.InvalidParam("The request body did not contain an allowed value of argument 'field'. Allowed values are either: 'avatar_url', 'displayname'.")
		}
	} else {
		res = eventutil.UserProfile{
			AvatarURL:   profile.AvatarURL,
			DisplayName: profile.DisplayName,
		}
	}

	return util.JSONResponse{
		Code: code,
		JSON: res,
	}
}
