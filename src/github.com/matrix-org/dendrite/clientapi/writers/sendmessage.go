package writers

import (
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/util"
)

type SendMessage struct {
}

func (s *SendMessage) OnIncomingRequest(req *http.Request) (interface{}, *util.HTTPError) {
	logger := req.Context().Value(util.CtxValueLogger).(*log.Entry)
	logger.Info("Doing stuff...")
	return nil, &util.HTTPError{
		Code:    404,
		Message: "Not implemented yet",
	}
}
