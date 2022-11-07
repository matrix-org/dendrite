package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v4"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/util"
)

// LoginTypeToken describes how to authenticate with a login token.
type LoginTypeTokenJwt struct {
	// UserAPI uapi.LoginTokenInternalAPI
	Config *config.ClientAPI
}

// Name implements Type.
func (t *LoginTypeTokenJwt) Name() string {
	return authtypes.LoginTypeJwt
}

type Claims struct {
	jwt.StandardClaims
}

const mIdUser = "m.id.user"

// LoginFromJSON implements Type. The cleanup function deletes the token from
// the database on success.
func (t *LoginTypeTokenJwt) LoginFromJSON(ctx context.Context, reqBytes []byte) (*Login, LoginCleanupFunc, *util.JSONResponse) {
	var r loginTokenRequest
	if err := httputil.UnmarshalJSON(reqBytes, &r); err != nil {
		return nil, nil, err
	}

	if r.Token == "" {
		return nil, nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Token field for JWT is missing"),
		}
	}
	c := &Claims{}
	token, err := jwt.ParseWithClaims(r.Token, c, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
		}
		return t.Config.JwtConfig.SecretKey, nil
	})

	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("jwt.ParseWithClaims failed")
		return nil, nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Couldn't parse JWT"),
		}
	}

	if !token.Valid {
		return nil, nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Invalid JWT"),
		}
	}

	r.Login.Identifier.User = c.Subject
	r.Login.Identifier.Type = mIdUser

	return &r.Login, func(context.Context, *util.JSONResponse) {}, nil
}
