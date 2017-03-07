package writers

import (
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/util"
)

// SendMessage implements /rooms/{roomID}/send/{eventType}
func SendMessage(req *http.Request, roomID, eventType string) util.JSONResponse {
	logger := util.GetLogger(req.Context())
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return *resErr
	}
	logger.WithFields(log.Fields{
		"roomID":    roomID,
		"eventType": eventType,
		"userID":    userID,
	}).Info("Doing stuff...")
	return util.MessageResponse(404, "Not implemented yet")
}
