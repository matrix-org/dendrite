package routing

import (
	"io"
	"net/http"

	"github.com/element-hq/dendrite/clientapi/auth"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// Deactivate handles POST requests to /account/deactivate
func Deactivate(
	req *http.Request,
	userInteractiveAuth *auth.UserInteractive,
	accountAPI api.ClientUserAPI,
	deviceAPI *api.Device,
) util.JSONResponse {
	ctx := req.Context()
	defer req.Body.Close() // nolint:errcheck
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The request body could not be read: " + err.Error()),
		}
	}

	login, errRes := userInteractiveAuth.Verify(ctx, bodyBytes, deviceAPI)
	if errRes != nil {
		return *errRes
	}

	localpart, serverName, err := gomatrixserverlib.SplitID('@', login.Username())
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	var res api.PerformAccountDeactivationResponse
	err = accountAPI.PerformAccountDeactivation(ctx, &api.PerformAccountDeactivationRequest{
		Localpart:  localpart,
		ServerName: serverName,
	}, &res)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("userAPI.PerformAccountDeactivation failed")
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
