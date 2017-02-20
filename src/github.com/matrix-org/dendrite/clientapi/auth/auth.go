package auth

import (
	"fmt"
	"net/http"
	"strings"
)

// VerifyAccessToken verifies that an access token was supplied in the given HTTP request
// and returns the user ID it corresponds to. Returns an error if there is no access token
// or the token is invalid.
func VerifyAccessToken(req *http.Request) (userID string, err error) {
	_, tokenErr := extractAccessToken(req)
	if tokenErr != nil {
		// err = MatrixError(MatrixError.M_MISSING_TOKEN, tokenErr.Error())
		return
	}
	// TODO: Do something with the token
	return
}

// extractAccessToken from a request, or return an error detailing what went wrong.
func extractAccessToken(req *http.Request) (string, error) {
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
