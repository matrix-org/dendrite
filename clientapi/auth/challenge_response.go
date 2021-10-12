package auth

import (
	"context"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"net/http"
)

type GetAccountByChallengeResponse func(ctx context.Context, localpart, b64encodedSignature, challenge string) (*api.Account, error)

type ChallengeResponseRequest struct {
	Login
	Signature string `json:"signature"`
}

// LoginTypeChallengeResponse using public key encryption
type LoginTypeChallengeResponse struct {
	GetAccountByChallengeResponse GetAccountByChallengeResponse
	Config                        *config.ClientAPI
}

func (t *LoginTypeChallengeResponse) Name() string {
	return authtypes.LoginTypeChallengeResponse
}

func (t *LoginTypeChallengeResponse) Request() interface{} {
	return &ChallengeResponseRequest{}
}

func (t *LoginTypeChallengeResponse) Login(ctx context.Context, req interface{}, challenge string) (*Login, *util.JSONResponse) {
	r := req.(*ChallengeResponseRequest)
	username := r.Username()
	if username == "" {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("A username must be supplied."),
		}
	}
	localpart, err := userutil.ParseUsernameParam(username, &t.Config.Matrix.ServerName)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
	}
	if r.Signature == "" {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidSignature("No signature provided"),
		}
	}
	_, err = t.GetAccountByChallengeResponse(ctx, localpart, r.Signature, challenge)
	if err != nil {
		// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
		// but that would leak the existence of the user.
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The digital signature is incorrect or the account does not exist."),
		}
	}
	return &r.Login, nil
}
