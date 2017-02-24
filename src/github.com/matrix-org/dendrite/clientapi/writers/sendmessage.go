package writers

import (
	"net/http"

	"github.com/matrix-org/util"
)

// SendMessage implements /rooms/{roomID}/send/{eventType}
func SendMessage(req *http.Request, roomID, eventType string) util.JSONResponse {
	logger := util.GetLogger(req.Context())
	logger.WithField("roomID", roomID).WithField("eventType", eventType).Info("Doing stuff...")
	return util.MessageResponse(404, "Not implemented yet")
}
