package writers

import (
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

// SendMessage implements /rooms/{roomID}/send/{eventType}
func SendMessage(req *http.Request, roomID, eventType string) util.JSONResponse {
	logger := util.GetLogger(req.Context())
	userID, resErr, err := auth.VerifyAccessToken(req)
	if err != nil {
		logger.WithError(err).Error("Failed to verify access token")
		return jsonerror.InternalServerError()
	}
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
