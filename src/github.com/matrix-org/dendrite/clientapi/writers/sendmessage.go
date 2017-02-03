package writers

import (
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/util"
)

// SendMessage handles HTTP requests to /rooms/$room_id/send/$event_type
type SendMessage struct {
}

// OnIncomingRequest implements util.JSONRequestHandler
func (s *SendMessage) OnIncomingRequest(req *http.Request) (interface{}, *util.HTTPError) {
	logger := req.Context().Value(util.CtxValueLogger).(*log.Entry)
	logger.Info("Doing stuff...")
	return nil, &util.HTTPError{
		Code:    404,
		Message: "Not implemented yet",
	}
}
