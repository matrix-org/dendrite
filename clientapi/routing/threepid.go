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
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type reqTokenResponse struct {
	SID       string `json:"sid"`
	SumbitURL string `json:"submit_url,omitempty"`
}

type threePIDsResponse struct {
	ThreePIDs []authtypes.ThreePID `json:"threepids"`
}

// RequestEmailToken implements:
//     POST /account/3pid/email/requestToken
//     POST /register/email/requestToken
func RequestEmailToken(req *http.Request, accountDB accounts.Database, userAPI userapi.UserInternalAPI, cfg *config.ClientAPI) util.JSONResponse {
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

	if cfg.Derived.SendEmails {
		createSessionResp := userapi.CreateSessionResponse{}
		ctx := req.Context()
		path := mux.Vars(req)["path"]
		var sessionType userapi.ThreepidSessionType
		switch path {
		case "account/3pid":
			sessionType = userapi.AccountThreepid
		case "register":
			sessionType = userapi.Register
		}
		err = userAPI.CreateSession(ctx, &userapi.CreateSessionRequest{
			ClientSecret: body.Secret,
			NextLink:     body.NextLink,
			ThreePid:     body.Email,
			SendAttempt:  body.SendAttempt,
			SessionType:  sessionType,
		}, &createSessionResp)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("userAPI.CreateSession failed")
			return jsonerror.InternalServerError()
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: createSessionResp,
		}
	} else {
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

}

func SubmitToken(req *http.Request, userAPI userapi.UserInternalAPI) util.JSONResponse {
	ctx := req.Context()
	validateSessionReq, matrixErr := parseSumbitTokenQuery(req.URL)
	if matrixErr != nil {
		util.GetLogger(req.Context()).WithError(matrixErr).Error("parseSumbitTokenQuery")
		return jsonerror.InternalServerError()
	}
	res := userapi.ValidateSessionResponse{}
	err := userAPI.ValidateSession(ctx, validateSessionReq, &res)
	if err == userapi.ErrBadSession {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(err.Error()),
		}
	}
	if res.NextLink != "" {
		return util.JSONResponse{
			Code: http.StatusFound,
			Headers: map[string]string{
				"Location": res.NextLink,
			},
		}
	} else {
		return util.JSONResponse{
			Code: http.StatusOK,
		}
	}
}

func parseSumbitTokenQuery(u *url.URL) (*userapi.ValidateSessionRequest, *jsonerror.MatrixError) {
	q := u.Query()
	sid := q["sid"]
	if sid == nil {
		return nil, jsonerror.MissingParam("sid param missing")
	}
	if len(sid) != 1 {
		return nil, jsonerror.InvalidParam("sid param malformed")
	}
	sidParsed, err := strconv.Atoi(sid[0])
	if err != nil {
		return nil, jsonerror.InvalidParam("sid is not an number")
	}
	clientSecret := q["client_secret"]
	if clientSecret == nil {
		return nil, jsonerror.MissingParam("client_secret param missing")
	}
	if len(clientSecret) != 1 {
		return nil, jsonerror.InvalidParam("client_secret param malformed")
	}
	token := q["token"]
	if token == nil {
		return nil, jsonerror.MissingParam("token param missing")
	}
	if len(token) != 1 {
		return nil, jsonerror.InvalidParam("token param malformed")
	}
	return &userapi.ValidateSessionRequest{
		SessionOwnership: userapi.SessionOwnership{
			Sid:          int64(sidParsed),
			ClientSecret: clientSecret[0],
		},
		Token: token[0]}, nil
}

// RequestAccountPasswordEmailToken implements:
//     POST /account/password/email/requestToken
func RequestAccountPasswordEmailToken(req *http.Request, accountDB accounts.Database, userAPI userapi.UserInternalAPI, device *userapi.Device) util.JSONResponse {
	var body threepid.EmailAssociationRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	var resp reqTokenResponse
	var err error

	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}
	ctx := req.Context()
	associated, err := isThreePidAssociated(req.Context(), body.Email, localpart, accountDB)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("isThreePidAssociated failed")
		return jsonerror.InternalServerError()
	}

	if !associated {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidParam("threepid is not bound to this user"),
		}
	}

	res := userapi.CreateSessionResponse{}
	err = userAPI.CreateSession(
		ctx,
		&userapi.CreateSessionRequest{
			ClientSecret: body.Secret,
			NextLink:     body.NextLink,
			ThreePid:     body.Email,
			SendAttempt:  body.SendAttempt,
			SessionType:  api.AccountPassword,
		},
		&res)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("userapi.CreateSessionRequest failed")
		return jsonerror.InternalServerError()
	}
	resp.SID = strconv.Itoa(int(res.Sid))
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}

func isThreePidAssociated(ctx context.Context, threepid, localpart string, db accounts.Database) (bool, error) {
	threepids, err := db.GetThreePIDsForLocalpart(ctx, localpart)
	if err != nil {
		return false, err
	}
	for i := range threepids {
		if threepid == threepids[i].Address && threepids[i].Medium == "email" {
			return true, nil
		}
	}
	return false, nil
}

// CheckAndSave3PIDAssociation implements POST /account/3pid
func CheckAndSave3PIDAssociation(
	req *http.Request, accountDB accounts.Database, device *api.Device,
	cfg *config.ClientAPI, userAPI userapi.UserInternalAPI,
) util.JSONResponse {
	var body threepid.EmailAssociationCheckRequest
	if reqErr := httputil.UnmarshalJSONRequest(req, &body); reqErr != nil {
		return *reqErr
	}

	// Check if the association has been validated
	var verified bool
	var err error
	var address, medium string
	if cfg.Derived.SendEmails {
		var res userapi.IsSessionValidatedResponse
		var sid int
		sid, err = strconv.Atoi(body.Creds.SID)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidParam("sid must be of type integer"),
			}
		}
		err = userAPI.IsSessionValidated(req.Context(), &userapi.SessionOwnership{
			Sid:          int64(sid),
			ClientSecret: body.Creds.Secret,
		}, &res)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("userAPI.IsSessionValidated failed")
			return jsonerror.InternalServerError()
		}
		verified = res.Validated
		address = res.ThreePid
		medium = "email" // TODO handle msisdn as well

	} else {
		verified, address, medium, err = threepid.CheckAssociation(req.Context(), body.Creds, cfg)
		if err == threepid.ErrNotTrusted {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.NotTrusted(body.Creds.IDServer),
			}
		} else if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAssociation failed")
			return jsonerror.InternalServerError()
		}
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
