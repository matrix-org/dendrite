// Copyright 2024 New Vector Ltd.
// Copyright 2021 Dan Peleg <dan@globekeeper.com>
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"
	"net/url"

	"github.com/element-hq/dendrite/clientapi/httputil"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// GetPushers handles /_matrix/client/r0/pushers
func GetPushers(
	req *http.Request, device *userapi.Device,
	userAPI userapi.ClientUserAPI,
) util.JSONResponse {
	var queryRes userapi.QueryPushersResponse
	localpart, domain, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	err = userAPI.QueryPushers(req.Context(), &userapi.QueryPushersRequest{
		Localpart:  localpart,
		ServerName: domain,
	}, &queryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryPushers failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	for i := range queryRes.Pushers {
		queryRes.Pushers[i].SessionID = 0
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: queryRes,
	}
}

// SetPusher handles /_matrix/client/r0/pushers/set
// This endpoint allows the creation, modification and deletion of pushers for this user ID.
// The behaviour of this endpoint varies depending on the values in the JSON body.
func SetPusher(
	req *http.Request, device *userapi.Device,
	userAPI userapi.ClientUserAPI,
) util.JSONResponse {
	localpart, domain, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	body := userapi.PerformPusherSetRequest{}
	if resErr := httputil.UnmarshalJSONRequest(req, &body); resErr != nil {
		return *resErr
	}
	if len(body.AppID) > 64 {
		return invalidParam("length of app_id must be no more than 64 characters")
	}
	if len(body.PushKey) > 512 {
		return invalidParam("length of pushkey must be no more than 512 bytes")
	}
	uInt := body.Data["url"]
	if uInt != nil {
		u, ok := uInt.(string)
		if !ok {
			return invalidParam("url must be string")
		}
		if u != "" {
			var pushUrl *url.URL
			pushUrl, err = url.Parse(u)
			if err != nil {
				return invalidParam("malformed url passed")
			}
			if pushUrl.Scheme != "https" {
				return invalidParam("only https scheme is allowed")
			}
		}

	}
	body.Localpart = localpart
	body.ServerName = domain
	body.SessionID = device.SessionID
	err = userAPI.PerformPusherSet(req.Context(), &body, &struct{}{})
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("PerformPusherSet failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func invalidParam(msg string) util.JSONResponse {
	return util.JSONResponse{
		Code: http.StatusBadRequest,
		JSON: spec.InvalidParam(msg),
	}
}
