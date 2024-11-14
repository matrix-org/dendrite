// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/clientapi/threepid"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/userapi/api"
	userdb "github.com/element-hq/dendrite/userapi/storage"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type reqTokenResponse struct {
	SID string `json:"sid"`
}

type ThreePIDsResponse struct {
	ThreePIDs []authtypes.ThreePID `json:"threepids"`
}

// RequestEmailToken implements:
//
//	POST /account/3pid/email/requestToken
//	POST /register/email/requestToken
func RequestEmailToken(req *http.Request, threePIDAPI api.ClientUserAPI, cfg *config.ClientAPI, client *fclient.Client) util.JSONResponse {
	var body threepid.EmailAssociationRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	var resp reqTokenResponse
	var err error

	// Check if the 3PID is already in use locally
	res := &api.QueryLocalpartForThreePIDResponse{}
	err = threePIDAPI.QueryLocalpartForThreePID(req.Context(), &api.QueryLocalpartForThreePIDRequest{
		ThreePID: body.Email,
		Medium:   "email",
	}, res)

	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("threePIDAPI.QueryLocalpartForThreePID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if len(res.Localpart) > 0 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MatrixError{
				ErrCode: spec.ErrorThreePIDInUse,
				Err:     userdb.Err3PIDInUse.Error(),
			},
		}
	}

	resp.SID, err = threepid.CreateSession(req.Context(), body, cfg, client)
	switch err.(type) {
	case nil:
	case threepid.ErrNotTrusted:
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CreateSession failed")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.NotTrusted(body.IDServer),
		}
	default:
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CreateSession failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}

// CheckAndSave3PIDAssociation implements POST /account/3pid
func CheckAndSave3PIDAssociation(
	req *http.Request, threePIDAPI api.ClientUserAPI, device *api.Device,
	cfg *config.ClientAPI, client *fclient.Client,
) util.JSONResponse {
	var body threepid.EmailAssociationCheckRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	// Check if the association has been validated
	verified, address, medium, err := threepid.CheckAssociation(req.Context(), body.Creds, cfg, client)
	switch err.(type) {
	case nil:
	case threepid.ErrNotTrusted:
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAssociation failed")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.NotTrusted(body.Creds.IDServer),
		}
	default:
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAssociation failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if !verified {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MatrixError{
				ErrCode: spec.ErrorThreePIDAuthFailed,
				Err:     "Failed to auth 3pid",
			},
		}
	}

	if body.Bind {
		// Publish the association on the identity server if requested
		err = threepid.PublishAssociation(req.Context(), body.Creds, device.UserID, cfg, client)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("threepid.PublishAssociation failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
	}

	// Save the association in the database
	localpart, domain, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if err = threePIDAPI.PerformSaveThreePIDAssociation(req.Context(), &api.PerformSaveThreePIDAssociationRequest{
		ThreePID:   address,
		Localpart:  localpart,
		ServerName: domain,
		Medium:     medium,
	}, &struct{}{}); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("threePIDAPI.PerformSaveThreePIDAssociation failed")
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

// GetAssociated3PIDs implements GET /account/3pid
func GetAssociated3PIDs(
	req *http.Request, threepidAPI api.ClientUserAPI, device *api.Device,
) util.JSONResponse {
	localpart, domain, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	res := &api.QueryThreePIDsForLocalpartResponse{}
	err = threepidAPI.QueryThreePIDsForLocalpart(req.Context(), &api.QueryThreePIDsForLocalpartRequest{
		Localpart:  localpart,
		ServerName: domain,
	}, res)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("threepidAPI.QueryThreePIDsForLocalpart failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: ThreePIDsResponse{res.ThreePIDs},
	}
}

// Forget3PID implements POST /account/3pid/delete
func Forget3PID(req *http.Request, threepidAPI api.ClientUserAPI) util.JSONResponse {
	var body authtypes.ThreePID
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	if err := threepidAPI.PerformForgetThreePID(req.Context(), &api.PerformForgetThreePIDRequest{
		ThreePID: body.Address,
		Medium:   body.Medium,
	}, &struct{}{}); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("threepidAPI.PerformForgetThreePID failed")
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
