package routing

import (
	"github.com/matrix-org/util"
	"net/http"
)

// represents response to an AppService URI to User Id
// (/_matrix/app/r0/user?uri={url_encoded_uri}) request
type URIToUIDResponse struct {
	UserID string `json:"user_id"`
}

// URIToUID implements `/_matrix/app/r0/user?uri={url_encoded_uri}`, which
// enables users to contact appservice users directly by taking an encoded
// URI and turning it into a Matrix ID on the homeserver.
// https://matrix.org/docs/spec/application_service/unstable.html#user-ids

func URIToUID(req *http.Request) util.JSONResponse {
	// TODO: Implement
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: nil,
	}
}
