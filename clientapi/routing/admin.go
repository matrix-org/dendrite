package routing

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/httputil"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

func AdminEvacuateRoom(req *http.Request, cfg *config.ClientAPI, device *userapi.Device, rsAPI roomserverAPI.ClientRoomserverAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}
	roomID, ok := vars["roomID"]
	if !ok {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Expecting room ID."),
		}
	}
	res := &roomserverAPI.PerformAdminEvacuateRoomResponse{}
	if err := rsAPI.PerformAdminEvacuateRoom(
		req.Context(),
		&roomserverAPI.PerformAdminEvacuateRoomRequest{
			RoomID: roomID,
		},
		res,
	); err != nil {
		return util.ErrorResponse(err)
	}
	if err := res.Error; err != nil {
		return err.JSONResponse()
	}
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"affected": res.Affected,
		},
	}
}

func AdminEvacuateUser(req *http.Request, cfg *config.ClientAPI, device *userapi.Device, rsAPI roomserverAPI.ClientRoomserverAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}
	userID, ok := vars["userID"]
	if !ok {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Expecting user ID."),
		}
	}
	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return util.MessageResponse(http.StatusBadRequest, err.Error())
	}
	if domain != cfg.Matrix.ServerName {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("User ID must belong to this server."),
		}
	}
	res := &roomserverAPI.PerformAdminEvacuateUserResponse{}
	if err := rsAPI.PerformAdminEvacuateUser(
		req.Context(),
		&roomserverAPI.PerformAdminEvacuateUserRequest{
			UserID: userID,
		},
		res,
	); err != nil {
		return jsonerror.InternalAPIError(req.Context(), err)
	}
	if err := res.Error; err != nil {
		return err.JSONResponse()
	}
	return util.JSONResponse{
		Code: 200,
		JSON: map[string]interface{}{
			"affected": res.Affected,
		},
	}
}

func AdminResetPassword(req *http.Request, cfg *config.ClientAPI, device *userapi.Device, userAPI userapi.ClientUserAPI) util.JSONResponse {
	vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
	if err != nil {
		return util.ErrorResponse(err)
	}
	localpart, ok := vars["localpart"]
	if !ok {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Expecting user localpart."),
		}
	}
	request := struct {
		Password string `json:"password"`
	}{}
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return util.MessageResponse(http.StatusBadRequest, err.Error())
	}
	if request.Password == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Expecting non-empty password."),
		}
	}
	updateReq := &userapi.PerformPasswordUpdateRequest{
		Localpart:     localpart,
		Password:      request.Password,
		LogoutDevices: true,
	}
	updateRes := &userapi.PerformPasswordUpdateResponse{}
	if err := userAPI.PerformPasswordUpdate(req.Context(), updateReq, updateRes); err != nil {
		return util.MessageResponse(http.StatusBadRequest, err.Error())
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
