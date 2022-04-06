package routing

import (
	"io/ioutil"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// Deactivate handles POST requests to /account/deactivate
func Deactivate(
	req *http.Request,
	userInteractiveAuth *auth.UserInteractive,
	accountAPI api.UserAccountAPI,
	deviceAPI *api.Device,
) util.JSONResponse {
	ctx := req.Context()
	defer req.Body.Close() // nolint:errcheck
	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be read: " + err.Error()),
		}
	}

	login, errRes := userInteractiveAuth.Verify(ctx, bodyBytes, deviceAPI)
	if errRes != nil {
		return *errRes
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', login.Username())
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	var res api.PerformAccountDeactivationResponse
	err = accountAPI.PerformAccountDeactivation(ctx, &api.PerformAccountDeactivationRequest{
		Localpart: localpart,
	}, &res)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("userAPI.PerformAccountDeactivation failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
