package routing

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
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
}

func Password(
	req *http.Request,
	userAPI api.UserInternalAPI,
	device *api.Device,
	cfg *config.ClientAPI,
) util.JSONResponse {
	// Check that the existing password is right.
	var r newPasswordRequest
	r.LogoutDevices = true

	logrus.WithFields(logrus.Fields{
		"sessionId": device.SessionID,
		"userId":    device.UserID,
	}).Debug("Changing password")

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

	// Require password auth to change the password.
	if r.Auth.Type != authtypes.LoginTypePassword {
		return util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: newUserInteractiveResponse(
				sessionID,
				[]authtypes.Flow{
					{
						Stages: []authtypes.LoginType{authtypes.LoginTypePassword},
					},
				},
				nil,
			),
		}
	}

	// Check if the existing password is correct.
	typePassword := auth.LoginTypePassword{
		GetAccountByPassword: userAPI.QueryAccountByPassword,
		Config:               cfg,
	}
	if _, authErr := typePassword.Login(req.Context(), &r.Auth.PasswordRequest); authErr != nil {
		return *authErr
	}
	sessions.addCompletedSessionStage(sessionID, authtypes.LoginTypePassword)

	// Check the new password strength.
	if resErr = validatePassword(r.NewPassword); resErr != nil {
		return *resErr
	}

	// Get the local part.
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	// Ask the user API to perform the password change.
	passwordReq := &api.PerformPasswordUpdateRequest{
		Localpart: localpart,
		Password:  r.NewPassword,
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
		logoutReq := &api.PerformDeviceDeletionRequest{
			UserID:         device.UserID,
			DeviceIDs:      nil,
			ExceptDeviceID: device.ID,
		}
		logoutRes := &api.PerformDeviceDeletionResponse{}
		if err := userAPI.PerformDeviceDeletion(req.Context(), logoutReq, logoutRes); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("PerformDeviceDeletion failed")
			return jsonerror.InternalServerError()
		}

		pushersReq := &api.PerformPusherDeletionRequest{
			Localpart: localpart,
			SessionID: device.SessionID,
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
