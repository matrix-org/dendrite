package routing

import (
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/threepid"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type newPasswordRequest struct {
	NewPassword   string          `json:"new_password"`
	LogoutDevices bool            `json:"logout_devices"`
	Auth          newPasswordAuth `json:"auth"`
}

type newPasswordAuth struct {
	Type    string `json:"type"`
	Session string `json:"session"`
	auth.PasswordRequest
	ThreePidCreds threepid.Credentials `json:"threepid_creds"`
}

func Password(
	req *http.Request,
	userAPI api.ClientUserAPI,
	device *api.Device,
	cfg *config.ClientAPI,
) util.JSONResponse {
	// Check that the existing password is right.
	var fields logrus.Fields
	if device != nil {
		fields = logrus.Fields{
			"sessionId": device.SessionID,
			"userId":    device.UserID,
		}
	}
	var r newPasswordRequest
	r.LogoutDevices = true

	logrus.WithFields(fields).Debug("Changing password")

	// Unmarshal the request.
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	// Retrieve or generate the sessionID
	sessionID := r.Auth.Session
	if sessionID == "" {
		// Generate a new, random session ID
		sessionID = util.RandomString(sessionIDLength)
	}
	var localpart string
	var domain gomatrixserverlib.ServerName
	switch r.Auth.Type {
	case authtypes.LoginTypePassword:
		// Check if the existing password is correct.
		typePassword := auth.LoginTypePassword{
			UserApi: userAPI,
			Config:  cfg,
		}
		if _, authErr := typePassword.Login(req.Context(), &r.Auth.PasswordRequest); authErr != nil {
			return *authErr
		}
		// Get the local part.
		var err error
		localpart, domain, err = gomatrixserverlib.SplitID('@', device.UserID)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
			return jsonerror.InternalServerError()
		}
		sessions.addCompletedSessionStage(sessionID, authtypes.LoginTypePassword)
	case authtypes.LoginTypeEmail:
		threePid := &authtypes.ThreePID{}
		r.Auth.ThreePidCreds.IDServer = cfg.ThreePidDelegate
		var (
			bound bool
			err   error
		)
		bound, threePid.Address, threePid.Medium, err = threepid.CheckAssociation(req.Context(), r.Auth.ThreePidCreds, cfg)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("threepid.CheckAssociation failed")
			return jsonerror.InternalServerError()
		}
		if !bound {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.MatrixError{
					ErrCode: "M_THREEPID_AUTH_FAILED",
					Err:     "Failed to auth 3pid",
				},
			}
		}
		var res api.QueryLocalpartForThreePIDResponse
		err = userAPI.QueryLocalpartForThreePID(req.Context(), &api.QueryLocalpartForThreePIDRequest{
			Medium:   threePid.Medium,
			ThreePID: threePid.Address,
		}, &res)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("userAPI.QueryLocalpartForThreePID failed")
			return jsonerror.InternalServerError()
		}
		if res.Localpart == "" {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.MatrixError{
					ErrCode: "M_THREEPID_NOT_FOUND",
					Err:     "3pid is not bound to any account",
				},
			}
		}
		localpart = res.Localpart
		domain = res.ServerName
		sessions.addCompletedSessionStage(sessionID, authtypes.LoginTypeEmail)
	default:
		flows := []authtypes.Flow{
			{
				Stages: []authtypes.LoginType{authtypes.LoginTypePassword},
			},
		}
		if cfg.ThreePidDelegate != "" {
			flows = append(flows, authtypes.Flow{
				Stages: []authtypes.LoginType{authtypes.LoginTypeEmail},
			})
		}
		// Require password auth to change the password.
		if r.Auth.Type == authtypes.LoginTypePassword {
			return util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: newUserInteractiveResponse(
					sessionID,
					flows,
					nil,
				),
			}
		}
	}

	// Check the new password strength.
	if err := internal.ValidatePassword(r.NewPassword); err != nil {
		return *internal.PasswordResponse(err)
	}

	// Ask the user API to perform the password change.
	passwordReq := &api.PerformPasswordUpdateRequest{
		Localpart:  localpart,
		ServerName: domain,
		Password:   r.NewPassword,
	}
	passwordRes := &api.PerformPasswordUpdateResponse{}
	if err := userAPI.PerformPasswordUpdate(req.Context(), passwordReq, passwordRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("PerformPasswordUpdate failed")
		return jsonerror.InternalServerError()
	}
	if !passwordRes.PasswordUpdated {
		util.GetLogger(req.Context()).Error("Expected password to have been updated but wasn't")
		return jsonerror.InternalServerError()
	}

	// If the request asks us to log out all other devices then
	// ask the user API to do that.

	if r.LogoutDevices {
		var logoutReq *api.PerformDeviceDeletionRequest
		var sessionId int64
		if device == nil {
			logoutReq = &api.PerformDeviceDeletionRequest{
				UserID:    fmt.Sprintf("@%s:%s", localpart, cfg.Matrix.ServerName),
				DeviceIDs: []string{},
			}
			sessionId = 0
		} else {
			logoutReq = &api.PerformDeviceDeletionRequest{
				UserID:         device.UserID,
				DeviceIDs:      nil,
				ExceptDeviceID: device.ID,
			}
			sessionId = device.SessionID
		}
		logoutRes := &api.PerformDeviceDeletionResponse{}
		if err := userAPI.PerformDeviceDeletion(req.Context(), logoutReq, logoutRes); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("PerformDeviceDeletion failed")
			return jsonerror.InternalServerError()
		}

		pushersReq := &api.PerformPusherDeletionRequest{
			Localpart:  localpart,
			ServerName: domain,
			SessionID:  sessionId,
		}
		if err := userAPI.PerformPusherDeletion(req.Context(), pushersReq, &struct{}{}); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("PerformPusherDeletion failed")
			return jsonerror.InternalServerError()
		}
	}

	// Return a success code.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
