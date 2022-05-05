package routing

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/httputil"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

func AdminEvacuateRoom(req *http.Request, device *userapi.Device, rsAPI roomserverAPI.ClientRoomserverAPI) util.JSONResponse {
	if device.AccountType != userapi.AccountTypeAdmin {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("This API can only be used by admin users."),
		}
	}
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
	rsAPI.PerformAdminEvacuateRoom(
		req.Context(),
		&roomserverAPI.PerformAdminEvacuateRoomRequest{
			RoomID: roomID,
		},
		res,
	)
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
