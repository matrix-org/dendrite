package writers

import (
	"net/http"

	"github.com/matrix-org/util"
)

// SendMessage implements /rooms/{roomID}/send/{eventType}
func SendMessage(req *http.Request, roomID, eventType string) (interface{}, *util.HTTPError) {
	logger := util.GetLogger(req.Context())
	logger.WithField("roomID", roomID).WithField("eventType", eventType).Info("Doing stuff...")
	return nil, &util.HTTPError{
		Code:    404,
		Message: "Not implemented yet",
	}
}
