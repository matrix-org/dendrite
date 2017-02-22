package readers

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/util"
)

// Sync implements /sync
func Sync(req *http.Request) (interface{}, *util.HTTPError) {
	logger := util.GetLogger(req.Context())
	userID, err := auth.VerifyAccessToken(req)
	if err != nil {
		return nil, &util.HTTPError{
			Code: 403,
			JSON: err,
		}
	}

	logger.WithField("userID", userID).Info("Doing stuff...")
	return nil, &util.HTTPError{
		Code:    404,
		Message: "Not implemented yet",
	}
}
