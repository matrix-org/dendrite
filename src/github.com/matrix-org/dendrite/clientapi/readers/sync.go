package readers

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/util"
)

// Sync implements /sync
func Sync(req *http.Request) util.JSONResponse {
	logger := util.GetLogger(req.Context())
	userID, err := auth.VerifyAccessToken(req)
	if err != nil {
		return util.JSONResponse{
			Code: 403,
			JSON: err,
		}
	}

	logger.WithField("userID", userID).Info("Doing stuff...")
	return util.MessageResponse(404, "Not implemented yet")
}
