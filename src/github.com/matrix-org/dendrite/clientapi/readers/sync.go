package readers

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

// Sync implements /sync
func Sync(req *http.Request) util.JSONResponse {
	logger := util.GetLogger(req.Context())
	userID, resErr, err := auth.VerifyAccessToken(req)
	if err != nil {
		logger.WithError(err).Error("Failed to verify access token")
		return jsonerror.InternalServerError()
	}
	if resErr != nil {
		return *resErr
	}

	logger.WithField("userID", userID).Info("Doing stuff...")
	return util.MessageResponse(404, "Not implemented yet")
}
