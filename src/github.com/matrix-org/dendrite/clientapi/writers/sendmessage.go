package writers

import (
	"net/http"

	"github.com/matrix-org/util"
)

// SendMessage handles HTTP requests to /rooms/$room_id/send/$event_type
type SendMessage struct {
}

// OnIncomingRequest implements /rooms/{roomID}/send/{eventType}
func (s *SendMessage) OnIncomingRequest(req *http.Request, roomID, eventType string) (interface{}, *util.HTTPError) {
	logger := util.GetLogger(req.Context())
	logger.WithField("roomID", roomID).WithField("eventType", eventType).Info("Doing stuff...")
	return nil, &util.HTTPError{
		Code:    404,
		Message: "Not implemented yet",
	}
}
