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

package readers

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/threepid"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type reqTokenResponse struct {
	SID string `json:"sid"`
}

type threePIDsResponse struct {
	ThreePIDs []threePID `json:"threepids"`
}

type threePID struct {
	Medium  string `json:"medium"`
	Address string `json:"address"`
}

// Request3PIDToken implements:
//     POST /account/3pid/email/requestToken
//     POST /register/email/requestToken
func Request3PIDToken(req *http.Request, accountDB *accounts.Database) util.JSONResponse {
	var body threepid.EmailAssociationRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	var resp reqTokenResponse
	var err error

	// Check if the 3PID is already in use locally
	localpart, err := accountDB.GetLocalpartForThreePID(body.Email)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if len(localpart) > 0 {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.MatrixError{
				ErrCode: "M_THREEPID_IN_USE",
				Err:     accounts.Err3PIDInUse.Error(),
			},
		}
	}

	resp.SID, err = threepid.CreateSession(body)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: resp,
	}
}

// CheckAndSave3PIDAssociation implements POST /account/3pid
func CheckAndSave3PIDAssociation(
	req *http.Request, accountDB *accounts.Database, device *authtypes.Device,
) util.JSONResponse {
	var body threepid.EmailAssociationCheckRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	verified, address, err := threepid.CheckAssociation(body.Creds)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if !verified {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.MatrixError{
				ErrCode: "M_THREEPID_AUTH_FAILED",
				Err:     "Failed to auth 3pid",
			},
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if err = accountDB.SaveThreePIDAssociation(address, localpart); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

// GetAssociated3PIDs implements GET /account/3pid
func GetAssociated3PIDs(
	req *http.Request, accountDB *accounts.Database, device *authtypes.Device,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	threepids, err := accountDB.GetThreePIDsForLocalpart(localpart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	var resp threePIDsResponse
	for address, medium := range threepids {
		tpid := threePID{Medium: medium, Address: address}
		resp.ThreePIDs = append(resp.ThreePIDs, tpid)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: resp,
	}
}

// Forget3PID implements POST /account/3pid/delete
func Forget3PID(req *http.Request, accountDB *accounts.Database) util.JSONResponse {
	var body threePID
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	if err := accountDB.RemoveThreePIDAssociation(body.Address); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}
