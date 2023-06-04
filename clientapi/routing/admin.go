package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/internal/httputil"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/userapi/api"
)

func AdminCreateNewRegistrationToken(req *http.Request, cfg *config.ClientAPI, rsAPI roomserverAPI.ClientRoomserverAPI) util.JSONResponse {
	if !cfg.RegistrationRequiresToken {
		return util.MatrixErrorResponse(
			http.StatusForbidden,
			string(spec.ErrorForbidden),
			"Registration via tokens is not enabled on this homeserver",
		)
	}
	request := struct {
		Token       string `json:"token"`
		UsesAllowed int32  `json:"uses_allowed"`
		ExpiryTime  int64  `json:"expiry_time"`
		Length      int32  `json:"length"`
	}{}

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return util.MatrixErrorResponse(
			http.StatusBadRequest,
			string(spec.ErrorBadJSON),
			"Failed to decode request body:",
		)
	}
	token := request.Token
	if len(token) == 0 || len(token) > 64 {
		return util.MatrixErrorResponse(
			http.StatusBadRequest,
			string(spec.ErrorInvalidParam),
			"token must not be empty and must not be longer than 64")
	}
	isTokenValid, _ := regexp.MatchString("^[[:ascii:][:digit:]_]*$", token)
	if !isTokenValid {
		return util.MatrixErrorResponse(
			http.StatusBadRequest,
			string(spec.ErrorInvalidParam),
			"token must consist only of characters matched by the regex [A-Za-z0-9-_]")
	}
	length := request.Length
	if !(length > 0 && length <= 64) {
		return util.MatrixErrorResponse(
			http.StatusBadRequest,
			string(spec.ErrorInvalidParam),
			"length must be greater than zero and not greater than 64")
	}
	// TODO: Generate Random Token
	// token = GenerateRandomToken(length)
	usesAllowed := request.UsesAllowed
	if usesAllowed < 0 {
		return util.MatrixErrorResponse(
			http.StatusBadRequest,
			string(spec.ErrorInvalidParam),
			"uses_allowed must be a non-negative integer or null")
	}

	expiryTime := request.ExpiryTime
	if expiryTime != 0 && expiryTime < time.Now().UnixNano()/int64(time.Millisecond) {
		return util.MatrixErrorResponse(
			http.StatusBadRequest,
			string(spec.ErrorInvalidParam),
			"expiry_time must not be in the past")
	}
	pending := 0
	completed := 0
	created, err := rsAPI.PerformAdminCreateRegistrationToken(req.Context(), token, usesAllowed, int32(pending), int32(completed), expiryTime)
	if err != nil {
		return util.MatrixErrorResponse(
			http.StatusInternalServerError,
			string(spec.ErrorUnknown),
			err.Error(),
		)
	}
	if !created {
		return util.MatrixErrorResponse(
			http.StatusBadRequest,
			string(spec.ErrorInvalidParam),
			fmt.Sprintf("Token alreaady exists: %s", token))
	}
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"token":        token,
			"uses_allowed": usesAllowed,
			"pending":      pending,
			"completed":    completed,
			"expiry_time":  expiryTime,
		},
	}
}

func AdminEvacuateRoom(req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}

	affected, err := rsAPI.PerformAdminEvacuateRoom(req.Context(), vars["roomID"])
	switch err.(type) {
	case nil:
	case eventutil.ErrRoomNoExists:
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound(err.Error()),
		}
	default:
		logrus.WithError(err).WithField("roomID", vars["roomID"]).Error("Failed to evacuate room")
		return util.ErrorResponse(err)
	}
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"affected": affected,
		},
	}
}

func AdminEvacuateUser(req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}

	affected, err := rsAPI.PerformAdminEvacuateUser(req.Context(), vars["userID"])
	if err != nil {
		logrus.WithError(err).WithField("userID", vars["userID"]).Error("Failed to evacuate user")
		return util.MessageResponse(http.StatusBadRequest, err.Error())
	}

	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"affected": affected,
		},
	}
}

func AdminPurgeRoom(req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}

	if err = rsAPI.PerformAdminPurgeRoom(context.Background(), vars["roomID"]); err != nil {
		return util.ErrorResponse(err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

func AdminResetPassword(req *http.Request, cfg *config.ClientAPI, device *api.Device, userAPI api.ClientUserAPI) util.JSONResponse {
	if req.Body == nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("Missing request body"),
		}
	}
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}
	var localpart string
	userID := vars["userID"]
	localpart, serverName, err := cfg.Matrix.SplitLocalID('@', userID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(err.Error()),
		}
	}
	accAvailableResp := &api.QueryAccountAvailabilityResponse{}
	if err = userAPI.QueryAccountAvailability(req.Context(), &api.QueryAccountAvailabilityRequest{
		Localpart:  localpart,
		ServerName: serverName,
	}, accAvailableResp); err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if accAvailableResp.Available {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.Unknown("User does not exist"),
		}
	}
	request := struct {
		Password      string `json:"password"`
		LogoutDevices bool   `json:"logout_devices"`
	}{}
	if err = json.NewDecoder(req.Body).Decode(&request); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("Failed to decode request body: " + err.Error()),
		}
	}
	if request.Password == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MissingParam("Expecting non-empty password."),
		}
	}

	if err = internal.ValidatePassword(request.Password); err != nil {
		return *internal.PasswordResponse(err)
	}

	updateReq := &api.PerformPasswordUpdateRequest{
		Localpart:     localpart,
		ServerName:    serverName,
		Password:      request.Password,
		LogoutDevices: request.LogoutDevices,
	}
	updateRes := &api.PerformPasswordUpdateResponse{}
	if err := userAPI.PerformPasswordUpdate(req.Context(), updateReq, updateRes); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("Failed to perform password update: " + err.Error()),
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct {
			Updated bool `json:"password_updated"`
		}{
			Updated: updateRes.PasswordUpdated,
		},
	}
}

func AdminReindex(req *http.Request, cfg *config.ClientAPI, device *api.Device, natsClient *nats.Conn) util.JSONResponse {
	_, err := natsClient.RequestMsg(nats.NewMsg(cfg.Matrix.JetStream.Prefixed(jetstream.InputFulltextReindex)), time.Second*10)
	if err != nil {
		logrus.WithError(err).Error("failed to publish nats message")
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

func AdminMarkAsStale(req *http.Request, cfg *config.ClientAPI, keyAPI api.ClientKeyAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}
	userID := vars["userID"]

	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return util.MessageResponse(http.StatusBadRequest, err.Error())
	}
	if cfg.Matrix.IsLocalServerName(domain) {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("Can not mark local device list as stale"),
		}
	}

	err = keyAPI.PerformMarkAsStaleIfNeeded(req.Context(), &api.PerformMarkAsStaleRequest{
		UserID: userID,
		Domain: domain,
	}, &struct{}{})
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown(fmt.Sprintf("Failed to mark device list as stale: %s", err)),
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func AdminDownloadState(req *http.Request, device *api.Device, rsAPI roomserverAPI.ClientRoomserverAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}
	roomID, ok := vars["roomID"]
	if !ok {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MissingParam("Expecting room ID."),
		}
	}
	serverName, ok := vars["serverName"]
	if !ok {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MissingParam("Expecting remote server name."),
		}
	}
	if err = rsAPI.PerformAdminDownloadState(req.Context(), roomID, device.UserID, spec.ServerName(serverName)); err != nil {
		if errors.Is(err, eventutil.ErrRoomNoExists{}) {
			return util.JSONResponse{
				Code: 200,
				JSON: spec.NotFound(err.Error()),
			}
		}
		logrus.WithError(err).WithFields(logrus.Fields{
			"userID":     device.UserID,
			"serverName": serverName,
			"roomID":     roomID,
		}).Error("failed to download state")
		return util.ErrorResponse(err)
	}
	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}
