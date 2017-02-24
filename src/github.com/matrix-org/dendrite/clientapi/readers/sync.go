package readers

import (
	"net/http"

	"github.com/matrix-org/util"
)

// Sync implements /sync
func Sync(req *http.Request) util.JSONResponse {
	logger := util.GetLogger(req.Context())
	logger.Info("Doing stuff...")
	return util.MessageResponse(404, "Not implemented yet")
}
