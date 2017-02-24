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
	userID, err := auth.VerifyAccessToken(req)
	if err != nil {
		return util.JSONResponse{
			Code: 403,
			JSON: err,
		}
	}
	logger.WithFields(log.Fields{
		"roomID":    roomID,
		"eventType": eventType,
		"userID":    userID,
	}).Info("Doing stuff...")
	return util.MessageResponse(404, "Not implemented yet")
}
