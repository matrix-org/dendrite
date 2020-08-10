// Copyright 2017 Vector Creations Ltd
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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type reqTokenResponse struct {
	SID string `json:"sid"`
}

type threePIDsResponse struct {
	ThreePIDs []authtypes.ThreePID `json:"threepids"`
}

// RequestEmailToken implements:
//     POST /account/3pid/email/requestToken
//     POST /register/email/requestToken
func RequestEmailToken(req *http.Request, accountDB accounts.Database, cfg *config.ClientAPI) util.JSONResponse {
	var body threepid.EmailAssociationRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	var resp reqTokenResponse
	var err error

	// Check if the 3PID is already in use locally
	localpart, err := accountDB.GetLocalpartForThreePID(req.Context(), body.Email, "email")
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("accountDB.GetLocalpartForThreePID failed")
		return jsonerror.InternalServerError()
	}

	if len(localpart) > 0 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MatrixError{
				ErrCode: "M_THREEPID_IN_USE",
				Err:     accounts.Err3PIDInUse.Error(),
			},
		}
	}

	resp.SID, err = threepid.CreateSession(req.Context(), body, cfg)
	if err == threepid.ErrNotTrusted {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotTrusted(body.IDServer),
		}
	} else if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CreateSession failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}

// CheckAndSave3PIDAssociation implements POST /account/3pid
func CheckAndSave3PIDAssociation(
	req *http.Request, accountDB accounts.Database, device *api.Device,
	cfg *config.ClientAPI,
) util.JSONResponse {
	var body threepid.EmailAssociationCheckRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	// Check if the association has been validated
	verified, address, medium, err := threepid.CheckAssociation(req.Context(), body.Creds, cfg)
	if err == threepid.ErrNotTrusted {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotTrusted(body.Creds.IDServer),
		}
	} else if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAssociation failed")
		return jsonerror.InternalServerError()
	}

	if !verified {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MatrixError{
				ErrCode: "M_THREEPID_AUTH_FAILED",
				Err:     "Failed to auth 3pid",
			},
		}
	}

	if body.Bind {
		// Publish the association on the identity server if requested
		err = threepid.PublishAssociation(body.Creds, device.UserID, cfg)
		if err == threepid.ErrNotTrusted {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.NotTrusted(body.Creds.IDServer),
			}
		} else if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("threepid.PublishAssociation failed")
			return jsonerror.InternalServerError()
		}
	}

	// Save the association in the database
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	if err = accountDB.SaveThreePIDAssociation(req.Context(), address, localpart, medium); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("accountsDB.SaveThreePIDAssociation failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// GetAssociated3PIDs implements GET /account/3pid
func GetAssociated3PIDs(
	req *http.Request, accountDB accounts.Database, device *api.Device,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	threepids, err := accountDB.GetThreePIDsForLocalpart(req.Context(), localpart)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("accountDB.GetThreePIDsForLocalpart failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: threePIDsResponse{threepids},
	}
}

// Forget3PID implements POST /account/3pid/delete
func Forget3PID(req *http.Request, accountDB accounts.Database) util.JSONResponse {
	var body authtypes.ThreePID
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	if err := accountDB.RemoveThreePIDAssociation(req.Context(), body.Address, body.Medium); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("accountDB.RemoveThreePIDAssociation failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
