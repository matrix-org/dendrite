package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/eventutil"
	"github.com/gorilla/mux"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/constraints"

	clientapi "github.com/element-hq/dendrite/clientapi/api"
	"github.com/element-hq/dendrite/internal/httputil"
	roomserverAPI "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/userapi/api"
	userapi "github.com/element-hq/dendrite/userapi/api"
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
		UsesAllowed *int32 `json:"uses_allowed,omitempty"`
		ExpiryTime  *int64 `json:"expiry_time,omitempty"`
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
		if length <= 0 || length > 64 {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("length must be greater than zero and not greater than 64"),
			}
		}
		token = util.RandomString(int(length))
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
	if usesAllowed != nil && *usesAllowed < 0 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("uses_allowed must be a non-negative integer or null"),
		}
	}
	if expiryTime != nil && spec.Timestamp(*expiryTime).Time().Before(time.Now()) {
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
		UsesAllowed: usesAllowed,
		Pending:     &pending,
		Completed:   &completed,
		ExpiryTime:  expiryTime,
	}
	created, err := userAPI.PerformAdminCreateRegistrationToken(req.Context(), registrationToken)
	if !created {
		return util.JSONResponse{
			Code: http.StatusConflict,
			JSON: map[string]string{
				"error": fmt.Sprintf("token: %s already exists", token),
			},
		}
	}
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: err,
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"token":        token,
			"uses_allowed": getReturnValue(usesAllowed),
			"pending":      pending,
			"completed":    completed,
			"expiry_time":  getReturnValue(expiryTime),
		},
	}
}

func getReturnValue[t constraints.Integer](in *t) any {
	if in == nil {
		return nil
	}
	return *in
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
			Code: http.StatusInternalServerError,
			JSON: err,
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
	request := make(map[string]*int64)
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
		if usesAllowed != nil && *usesAllowed < 0 {
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
		if expiryTime != nil && spec.Timestamp(*expiryTime).Time().Before(time.Now()) {
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

func AdminResetPassword(req *http.Request, cfg *config.ClientAPI, device *api.Device, userAPI userapi.ClientUserAPI) util.JSONResponse {
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

func AdminMarkAsStale(req *http.Request, cfg *config.ClientAPI, keyAPI userapi.ClientKeyAPI) util.JSONResponse {
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

// GetEventReports returns reported events for a given user/room.
func GetEventReports(
	req *http.Request,
	rsAPI roomserverAPI.ClientRoomserverAPI,
	from, limit uint64,
	backwards bool,
	userID, roomID string,
) util.JSONResponse {

	eventReports, count, err := rsAPI.QueryAdminEventReports(req.Context(), from, limit, backwards, userID, roomID)
	if err != nil {
		logrus.WithError(err).Error("failed to query event reports")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	resp := map[string]any{
		"event_reports": eventReports,
		"total":         count,
	}

	// Add a next_token if there are still reports
	if int64(from+limit) < count {
		resp["next_token"] = int(from) + len(eventReports)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}

func GetEventReport(req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI, reportID string) util.JSONResponse {
	parsedReportID, err := strconv.ParseUint(reportID, 10, 64)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			// Given this is an admin endpoint, let them know what didn't work.
			JSON: spec.InvalidParam(err.Error()),
		}
	}

	report, err := rsAPI.QueryAdminEventReport(req.Context(), parsedReportID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown(err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: report,
	}
}

func DeleteEventReport(req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI, reportID string) util.JSONResponse {
	parsedReportID, err := strconv.ParseUint(reportID, 10, 64)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			// Given this is an admin endpoint, let them know what didn't work.
			JSON: spec.InvalidParam(err.Error()),
		}
	}

	err = rsAPI.PerformAdminDeleteEventReport(req.Context(), parsedReportID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown(err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

func parseUint64OrDefault(input string, defaultValue uint64) uint64 {
	v, err := strconv.ParseUint(input, 10, 64)
	if err != nil {
		return defaultValue
	}
	return v
}
