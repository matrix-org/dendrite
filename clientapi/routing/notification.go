// Copyright 2024 New Vector Ltd.
// Copyright 2021 Dan Peleg <dan@globekeeper.com>
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"
	"strconv"

	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// GetNotifications handles /_matrix/client/r0/notifications
func GetNotifications(
	req *http.Request, device *userapi.Device,
	userAPI userapi.ClientUserAPI,
) util.JSONResponse {
	var limit int64
	if limitStr := req.URL.Query().Get("limit"); limitStr != "" {
		var err error
		limit, err = strconv.ParseInt(limitStr, 10, 64)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("ParseInt(limit) failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
	}

	var queryRes userapi.QueryNotificationsResponse
	localpart, domain, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	err = userAPI.QueryNotifications(req.Context(), &userapi.QueryNotificationsRequest{
		Localpart:  localpart,
		ServerName: domain,
		From:       req.URL.Query().Get("from"),
		Limit:      int(limit),
		Only:       req.URL.Query().Get("only"),
	}, &queryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryNotifications failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	util.GetLogger(req.Context()).WithField("from", req.URL.Query().Get("from")).WithField("limit", limit).WithField("only", req.URL.Query().Get("only")).WithField("next", queryRes.NextToken).Infof("QueryNotifications: len %d", len(queryRes.Notifications))
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: queryRes,
	}
}
