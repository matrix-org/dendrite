package readers

import (
	"net/http"

	"github.com/matrix-org/util"
)

// Sync handles HTTP requests to /sync
type Sync struct{}

// OnIncomingRequest implements util.JSONRequestHandler
func (s *Sync) OnIncomingRequest(req *http.Request) (interface{}, *util.HTTPError) {
	logger := util.GetLogger(req.Context())
	logger.Info("Doing stuff...")
	return nil, &util.HTTPError{
		Code:    404,
		Message: "Not implemented yet",
	}
}
