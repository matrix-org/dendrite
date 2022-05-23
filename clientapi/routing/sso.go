// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package routing

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/sso"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// SSORedirect implements /login/sso/redirect
// https://spec.matrix.org/v1.2/client-server-api/#redirecting-to-the-authentication-server
func SSORedirect(
	req *http.Request,
	idpID string,
	cfg *config.ClientAPI,
) util.JSONResponse {
	if !cfg.Login.SSO.Enabled {
		return util.JSONResponse{
			Code: http.StatusNotImplemented,
			JSON: jsonerror.NotFound("authentication method disabled"),
		}
	}

	redirectURL := req.URL.Query().Get("redirectUrl")
	if redirectURL == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("redirectUrl parameter missing"),
		}
	}
	_, err := url.Parse(redirectURL)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("Invalid redirectURL: " + err.Error()),
		}
	}

	if idpID == "" {
		// Check configuration if the client didn't provide an ID.
		idpID = cfg.Login.SSO.DefaultProviderID
	}
	if idpID == "" && len(cfg.Login.SSO.Providers) > 0 {
		// Fall back to the first provider. If there are no providers, getProvider("") will fail.
		idpID = cfg.Login.SSO.Providers[0].ID
	}
	idpCfg, idpType := getProvider(cfg, idpID)
	if idpType == nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("unknown identity provider"),
		}
	}

	idpReq := &sso.IdentityProviderRequest{
		System:        idpCfg,
		CallbackURL:   req.URL.ResolveReference(&url.URL{Path: "../callback", RawQuery: url.Values{"provider": []string{idpID}}.Encode()}).String(),
		DendriteNonce: formatNonce(redirectURL),
	}
	u, err := idpType.AuthorizationURL(req.Context(), idpReq)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: err,
		}
	}

	resp := util.RedirectResponse(u)
	resp.Headers["Set-Cookie"] = (&http.Cookie{
		Name:     "oidc_nonce",
		Value:    idpReq.DendriteNonce,
		Expires:  time.Now().Add(10 * time.Minute),
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}).String()
	return resp
}

// SSOCallback implements /login/sso/callback.
// https://spec.matrix.org/v1.2/client-server-api/#handling-the-callback-from-the-authentication-server
func SSOCallback(
	req *http.Request,
	userAPI userAPIForSSO,
	cfg *config.ClientAPI,
) util.JSONResponse {
	ctx := req.Context()

	query := req.URL.Query()
	idpID := query.Get("provider")
	if idpID == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("provider parameter missing"),
		}
	}
	idpCfg, idpType := getProvider(cfg, idpID)
	if idpType == nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("unknown identity provider"),
		}
	}

	nonce, err := req.Cookie("oidc_nonce")
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("no nonce cookie: " + err.Error()),
		}
	}
	finalRedirectURL, err := parseNonce(nonce.Value)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: err,
		}
	}

	idpReq := &sso.IdentityProviderRequest{
		System: idpCfg,
		CallbackURL: (&url.URL{
			Scheme: req.URL.Scheme,
			Host:   req.URL.Host,
			Path:   req.URL.Path,
			RawQuery: url.Values{
				"provider": []string{idpID},
			}.Encode(),
		}).String(),
		DendriteNonce: nonce.Value,
	}
	result, err := idpType.ProcessCallback(ctx, idpReq, query)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: err,
		}
	}

	if result.Identifier == nil {
		// Not authenticated yet.
		return util.RedirectResponse(result.RedirectURL)
	}

	id, err := verifySSOUserIdentifier(ctx, userAPI, result.Identifier, cfg.Matrix.ServerName)
	if err != nil {
		util.GetLogger(ctx).WithError(err).WithField("identifier", result.Identifier).Error("failed to find user")
		return util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.Forbidden("ID not associated with a local account"),
		}
	}
	if id == nil {
		// The user doesn't exist.
		// TODO: let the user select a localpart and register an account.
		util.GetLogger(ctx).WithError(err).WithField("identifier", result.Identifier).Error("failed to find user")
		return util.JSONResponse{
			Code: http.StatusNotImplemented,
			JSON: jsonerror.Forbidden("SSO registration not implemented"),
		}
	}

	token, err := createLoginToken(ctx, userAPI, id)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Errorf("PerformLoginTokenCreation failed")
		return jsonerror.InternalServerError()
	}

	rquery := finalRedirectURL.Query()
	rquery.Set("loginToken", token.Token)
	resp := util.RedirectResponse(finalRedirectURL.ResolveReference(&url.URL{RawQuery: rquery.Encode()}).String())
	resp.Headers["Set-Cookie"] = (&http.Cookie{
		Name:   "oidc_nonce",
		Value:  "",
		MaxAge: -1,
		Secure: true,
	}).String()
	return resp
}

type userAPIForSSO interface {
	uapi.LoginTokenInternalAPI

	QueryLocalpartForSSO(ctx context.Context, req *uapi.QueryLocalpartForSSORequest, res *uapi.QueryLocalpartForSSOResponse) error
}

// getProvider looks up the given provider in the
// configuration. Returns nil if it wasn't found or was of unknown
// type.
func getProvider(cfg *config.ClientAPI, id string) (*config.IdentityProvider, sso.IdentityProvider) {
	for _, idp := range cfg.Login.SSO.Providers {
		if idp.ID == id {
			switch sso.IdentityProviderType(id) {
			case sso.TypeGitHub:
				return &idp, sso.GitHubIdentityProvider
			default:
				return nil, nil
			}
		}
	}
	return nil, nil
}

// formatNonce creates a random nonce that also contains the URL.
func formatNonce(redirectURL string) string {
	return util.RandomString(16) + "." + base64.RawURLEncoding.EncodeToString([]byte(redirectURL))
}

// parseNonce extracts the embedded URL from the nonce. The nonce
// should have been validated to be the original before calling this
// function. The URL is not integrity protected.
func parseNonce(s string) (redirectURL *url.URL, _ error) {
	if s == "" {
		return nil, jsonerror.MissingArgument("empty OIDC nonce cookie")
	}

	ss := strings.Split(s, ".")
	if len(ss) < 2 {
		return nil, jsonerror.InvalidArgumentValue("malformed OIDC nonce cookie")
	}

	urlbs, err := base64.RawURLEncoding.DecodeString(ss[1])
	if err != nil {
		return nil, jsonerror.InvalidArgumentValue("invalid redirect URL in OIDC nonce cookie")
	}
	u, err := url.Parse(string(urlbs))
	if err != nil {
		return nil, jsonerror.InvalidArgumentValue("invalid redirect URL in OIDC nonce cookie: " + err.Error())
	}

	return u, nil
}

// verifySSOUserIdentifier resolves an sso.UserIdentifier to a
// UserIdentifier using the User API. Returns nil if there is no
// associated user.
func verifySSOUserIdentifier(ctx context.Context, userAPI userAPIForSSO, id *sso.UserIdentifier, serverName gomatrixserverlib.ServerName) (*userutil.UserIdentifier, error) {
	req := &uapi.QueryLocalpartForSSORequest{
		Namespace: id.Namespace,
		Issuer:    id.Issuer,
		Subject:   id.Subject,
	}
	var res uapi.QueryLocalpartForSSOResponse
	if err := userAPI.QueryLocalpartForSSO(ctx, req, &res); err != nil {
		return nil, err
	}
	if res.Localpart == "" {
		return nil, nil
	}

	return &userutil.UserIdentifier{UserID: userutil.MakeUserID(res.Localpart, serverName)}, nil
}

func createLoginToken(ctx context.Context, userAPI userAPIForSSO, id *userutil.UserIdentifier) (*uapi.LoginTokenMetadata, error) {
	req := uapi.PerformLoginTokenCreationRequest{Data: uapi.LoginTokenData{UserID: id.UserID}}
	var resp uapi.PerformLoginTokenCreationResponse
	if err := userAPI.PerformLoginTokenCreation(ctx, &req, &resp); err != nil {
		return nil, err
	}
	return &resp.Metadata, nil
}
