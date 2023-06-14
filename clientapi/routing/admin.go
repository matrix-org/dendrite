package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"

	clientapi "github.com/matrix-org/dendrite/clientapi/api"
	"github.com/matrix-org/dendrite/internal/httputil"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/userapi/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

var validRegistrationTokenRegex = regexp.MustCompile("^[[:ascii:][:digit:]_]*$")

func AdminCreateNewRegistrationToken(req *http.Request, cfg *config.ClientAPI, userAPI userapi.ClientUserAPI) util.JSONResponse {
	if !cfg.RegistrationRequiresToken {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("Registration via tokens is not enabled on this homeserver"),
		}
	}
	request := struct {
		Token       string `json:"token"`
		UsesAllowed int32  `json:"uses_allowed"`
		ExpiryTime  int64  `json:"expiry_time"`
		Length      int32  `json:"length"`
	}{}

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(fmt.Sprintf("Failed to decode request body: %s", err)),
		}
	}

	token := request.Token
	usesAllowed := request.UsesAllowed
	expiryTime := request.ExpiryTime
	length := request.Length

	if len(token) == 0 {
		if length == 0 {
			// length not provided in request. Assign default value of 16.
			length = 16
		}
		// token not present in request body. Hence, generate a random token.
		if !(length > 0 && length <= 64) {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("length must be greater than zero and not greater than 64"),
			}
		}
		token = generateRandomToken(int(length))
	}

	if len(token) > 64 {
		//Token present in request body, but is too long.
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("token must not be longer than 64"),
		}
	}

	isTokenValid := validRegistrationTokenRegex.Match([]byte(token))
	if !isTokenValid {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("token must consist only of characters matched by the regex [A-Za-z0-9-_]"),
		}
	}
	// At this point, we have a valid token, either through request body or through random generation.

	if usesAllowed < 0 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("uses_allowed must be a non-negative integer or null"),
		}
	}

	if expiryTime != 0 && expiryTime < time.Now().UnixNano()/int64(time.Millisecond) {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("expiry_time must not be in the past"),
		}
	}
	pending := int32(0)
	completed := int32(0)
	// If usesAllowed or expiryTime is 0, it means they are not present in the request. NULL (indicating unlimited uses / no expiration will be persisted in DB)
	registrationToken := &clientapi.RegistrationToken{
		Token:       &token,
		UsesAllowed: &usesAllowed,
		Pending:     &pending,
		Completed:   &completed,
		ExpiryTime:  &expiryTime,
	}
	created, err := userAPI.PerformAdminCreateRegistrationToken(req.Context(), registrationToken)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: err,
		}
	}
	if !created {
		return util.JSONResponse{
			Code: http.StatusConflict,
			JSON: fmt.Sprintf("Token already exists: %s", token),
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"token":        token,
			"uses_allowed": getReturnValueForUsesAllowed(usesAllowed),
			"pending":      pending,
			"completed":    completed,
			"expiry_time":  getReturnValueExpiryTime(expiryTime),
		},
	}
}

func generateRandomToken(length int) string {
	allowedChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
	rand.Seed(time.Now().UnixNano())
	var sb strings.Builder
	for i := 0; i < length; i++ {
		randomIndex := rand.Intn(len(allowedChars))
		sb.WriteByte(allowedChars[randomIndex])
	}
	return sb.String()
}

func getReturnValueForUsesAllowed(usesAllowed int32) interface{} {
	if usesAllowed == 0 {
		return nil
	}
	return usesAllowed
}

func AdminListRegistrationTokens(req *http.Request, cfg *config.ClientAPI, userAPI userapi.ClientUserAPI) util.JSONResponse {
	queryParams := req.URL.Query()
	returnAll := true
	valid := true
	validQuery, ok := queryParams["valid"]
	if ok {
		returnAll = false
		validValue, err := strconv.ParseBool(validQuery[0])
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("invalid 'valid' query parameter"),
			}
		}
		valid = validValue
	}
	tokens, err := userAPI.PerformAdminListRegistrationTokens(req.Context(), returnAll, valid)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.ErrorUnknown,
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"registration_tokens": tokens,
		},
	}
}

func getReturnValueExpiryTime(expiryTime int64) interface{} {
	if expiryTime == 0 {
		return nil
	}
	return expiryTime
}

func AdminGetRegistrationToken(req *http.Request, cfg *config.ClientAPI, userAPI userapi.ClientUserAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}
	tokenText := vars["token"]
	token, err := userAPI.PerformAdminGetRegistrationToken(req.Context(), tokenText)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound(fmt.Sprintf("token: %s not found", tokenText)),
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: token,
	}
}

func AdminDeleteRegistrationToken(req *http.Request, cfg *config.ClientAPI, userAPI userapi.ClientUserAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}
	tokenText := vars["token"]
	err = userAPI.PerformAdminDeleteRegistrationToken(req.Context(), tokenText)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound(fmt.Sprintf("token: %s not found", tokenText)),
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{},
	}
}

func AdminUpdateRegistrationToken(req *http.Request, cfg *config.ClientAPI, userAPI userapi.ClientUserAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}
	tokenText := vars["token"]
	request := make(map[string]interface{})
	if err = json.NewDecoder(req.Body).Decode(&request); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(fmt.Sprintf("Failed to decode request body: %s", err)),
		}
	}
	newAttributes := make(map[string]interface{})
	usesAllowed, ok := request["uses_allowed"]
	if ok {
		// Only add usesAllowed to newAtrributes if it is present and valid
		// Non numeric values in payload will cause panic during type conversion. But this is the best way to mimic
		// Synapse's behaviour of updating the field if and only if it is present in request body.
		if !(usesAllowed == nil || int32(usesAllowed.(float64)) >= 0) {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("uses_allowed must be a non-negative integer or null"),
			}
		}
		newAttributes["usesAllowed"] = usesAllowed
	}
	expiryTime, ok := request["expiry_time"]
	if ok {
		// Only add expiryTime to newAtrributes if it is present and valid
		// Non numeric values in payload will cause panic during type conversion. But this is the best way to mimic
		// Synapse's behaviour of updating the field if and only if it is present in request body.
		if !(expiryTime == nil || int64(expiryTime.(float64)) > time.Now().UnixNano()/int64(time.Millisecond)) {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("expiry_time must not be in the past"),
			}
		}
		newAttributes["expiryTime"] = expiryTime
	}
	if len(newAttributes) == 0 {
		// No attributes to update. Return existing token
		return AdminGetRegistrationToken(req, cfg, userAPI)
	}
	updatedToken, err := userAPI.PerformAdminUpdateRegistrationToken(req.Context(), tokenText, newAttributes)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound(fmt.Sprintf("token: %s not found", tokenText)),
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: *updatedToken,
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
