package routing

import (
	"github.com/matrix-org/util"
	"net/http"
	"strings"
	"unicode"
)

// URIToUIDResponse represents response to an AppService URI to User Id
// (/_matrix/app/r0/user?uri={url_encoded_uri}) request
type URIToUIDResponse struct {
	UserID string `json:"user_id"`
}

// URIToUID implements `/_matrix/app/r0/user?uri={url_encoded_uri}`, which
// enables users to contact App Service users directly by taking an encoded
// URI and turning it into a Matrix ID on the homeserver.
// https://matrix.org/docs/spec/application_service/unstable.html#user-ids
// tel://123.1234 -> @tel_//123.1234:matrix.org
// mailto:test@matrix.org -> @mailto_test_matrix.org:matrix.org
func URIToUID(req *http.Request, cfg config.Dendrite) util.JSONResponse {
	homeserver := cfg.Matrix.ServerName
	uri := req.URL.Query().Get("uri")
	if uri == "" {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: nil,
		}
	}
	// V1 just replaces the illegal characters (from
	// https://matrix.org/docs/spec/appendices.html#id12) with _
	// TODO: Come up with a better way to turn URIs into User IDs
	w := strings.FieldsFunc(strings.ToLower(uri), func(r rune) bool {
		if unicode.IsDigit(r) || unicode.IsLetter(r) {
			return false
		}
		switch r {
		case '.', '_', '=', '-', '/':
			return false
		}
		return true
	})

	// compile user ID and return
	userID := "@" + strings.Join(w, "_") + ":" + homeserver

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: URIToUIDResponse{UserID: userID},
	}
}
