package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

// VerifyAccessToken verifies that an access token was supplied in the given HTTP request
// and returns the user ID it corresponds to. Returns resErr (an error response which can be
// sent to the client) if the token is invalid or there was a problem querying the database.
func VerifyAccessToken(req *http.Request) (userID string, resErr *util.JSONResponse) {
	token, tokenErr := extractAccessToken(req)
	if tokenErr != nil {
		resErr = &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.MissingToken(tokenErr.Error()),
		}
		return
	}
	if token == "fail" {
		res := util.ErrorResponse(fmt.Errorf("Fatal error"))
		resErr = &res
	}
	// TODO: Check the token against the database
	userID = token
	return
}

// extractAccessToken from a request, or return an error detailing what went wrong. The
// error message MUST be human-readable and comprehensible to the client.
func extractAccessToken(req *http.Request) (string, error) {
	// cf https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/api/auth.py#L631
	authBearer := req.Header.Get("Authorization")
	queryToken := req.URL.Query().Get("access_token")
	if authBearer != "" && queryToken != "" {
		return "", fmt.Errorf("mixing Authorization headers and access_token query parameters")
	}

	if queryToken != "" {
		return queryToken, nil
	}

	if authBearer != "" {
		parts := strings.SplitN(authBearer, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return "", fmt.Errorf("invalid Authorization header")
		}
		return parts[1], nil
	}

	return "", fmt.Errorf("missing access token")
}
