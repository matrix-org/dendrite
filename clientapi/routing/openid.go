package routing

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

type openIDTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	MatrixServerName string `json:"matrix_server_name"`
	ExpiresIn        int64  `json:"expires_in"`
}

// CreateOpenIDToken creates a new OpenID Connect token that a Matrix user
// can supply to an OpenID Relying Party
func CreateOpenIDToken(
	req *http.Request,
	userAPI api.UserInternalAPI,
	device *api.Device,
	userID, relyingParty string,
	cfg *config.ClientAPI,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("userID does not match the current user"),
		}
	}

	request := api.PerformOpenIDTokenCreationRequest{
		UserID:       userID,
		RelyingParty: relyingParty}
	response := api.PerformOpenIDTokenCreationResponse{}

	err := userAPI.PerformOpenIDTokenCreation(req.Context(), &request, &response)
	if err != nil {
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: openIDTokenResponse{
			AccessToken:      response.Token.Token,
			TokenType:        "Bearer",
			MatrixServerName: string(cfg.Matrix.ServerName),
			ExpiresIn:        response.Token.ExpiresTS / 1000, // convert ms to s
		},
	}
}
