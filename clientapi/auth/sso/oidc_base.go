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

package sso

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/tidwall/gjson"
)

type baseOIDCIdentityProvider struct {
	AuthURL                     *urlTemplate
	AccessTokenURL              *urlTemplate
	UserInfoURL                 *urlTemplate
	UserInfoAccept              string
	UserInfoEmailPath           string
	UserInfoSuggestedUserIDPath string
}

func (p *baseOIDCIdentityProvider) AuthorizationURL(ctx context.Context, req *IdentityProviderRequest) (string, error) {
	u, err := p.AuthURL.Execute(map[string]interface{}{
		"Config":      req.System,
		"State":       req.DendriteNonce,
		"RedirectURI": req.CallbackURL,
	}, url.Values{
		"client_id":     []string{req.System.OIDC.ClientID},
		"response_type": []string{"code"},
		"redirect_uri":  []string{req.CallbackURL},
		"state":         []string{req.DendriteNonce},
	})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (p *baseOIDCIdentityProvider) ProcessCallback(ctx context.Context, req *IdentityProviderRequest, values url.Values) (*CallbackResult, error) {
	state := values.Get("state")
	if state == "" {
		return nil, jsonerror.MissingArgument("state parameter missing")
	}
	if state != req.DendriteNonce {
		return nil, jsonerror.InvalidArgumentValue("state parameter not matching nonce")
	}

	if error := values.Get("error"); error != "" {
		if euri := values.Get("error_uri"); euri != "" {
			return &CallbackResult{RedirectURL: euri}, nil
		}

		desc := values.Get("error_description")
		if desc == "" {
			desc = error
		}
		switch error {
		case "unauthorized_client", "access_denied":
			return nil, jsonerror.Forbidden("SSO said no: " + desc)
		default:
			return nil, fmt.Errorf("SSO failed: %v", error)
		}
	}

	code := values.Get("code")
	if code == "" {
		return nil, jsonerror.MissingArgument("code parameter missing")
	}

	oidcAccessToken, err := p.getOIDCAccessToken(ctx, req, code)
	if err != nil {
		return nil, err
	}

	id, userID, err := p.getUserInfo(ctx, req, oidcAccessToken)
	if err != nil {
		return nil, err
	}

	return &CallbackResult{Identifier: id, SuggestedUserID: userID}, nil
}

func (p *baseOIDCIdentityProvider) getOIDCAccessToken(ctx context.Context, req *IdentityProviderRequest, code string) (string, error) {
	u, err := p.AccessTokenURL.Execute(nil, nil)
	if err != nil {
		return "", err
	}

	body := url.Values{
		"grant_type":   []string{"authorization_code"},
		"code":         []string{code},
		"redirect_uri": []string{req.CallbackURL},
		"client_id":    []string{req.System.OIDC.ClientID},
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	hreq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hreq.Header.Set("Accept", "application/x-www-form-urlencoded")

	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return "", err
	}
	defer hresp.Body.Close()

	ctype, _, err := mime.ParseMediaType(hresp.Header.Get("Content-Type"))
	if err != nil {
		return "", err
	}
	if ctype != "application/json" {
		return "", fmt.Errorf("expected URL encoded response, got content type %q", ctype)
	}

	var resp struct {
		TokenType   string `json:"token_type"`
		AccessToken string `json:"access_token"`

		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		ErrorURI         string `json:"error_uri"`
	}
	if err := json.NewDecoder(hresp.Body).Decode(&resp); err != nil {
		return "", err
	}

	if resp.Error != "" {
		desc := resp.ErrorDescription
		if desc == "" {
			desc = resp.Error
		}
		return "", fmt.Errorf("failed to retrieve OIDC access token: %s", desc)
	}

	if strings.ToLower(resp.TokenType) != "bearer" {
		return "", fmt.Errorf("expected bearer token, got type %q", resp.TokenType)
	}

	return resp.AccessToken, nil
}

func (p *baseOIDCIdentityProvider) getUserInfo(ctx context.Context, req *IdentityProviderRequest, oidcAccessToken string) (ssoUser *UserIdentifier, suggestedUserID string, _ error) {
	u, err := p.UserInfoURL.Execute(map[string]interface{}{
		"Config": req.System,
	}, nil)
	if err != nil {
		return nil, "", err
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}
	hreq.Header.Set("Authorization", "token "+oidcAccessToken)
	hreq.Header.Set("Accept", p.UserInfoAccept)

	hresp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return nil, "", err
	}
	defer hresp.Body.Close()

	ctype, _, err := mime.ParseMediaType(hresp.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", err
	}

	if ctype != "application/json" {
		return nil, "", fmt.Errorf("got unknown content type %q for user info", ctype)
	}

	body, err := ioutil.ReadAll(hresp.Body)
	if err != nil {
		return nil, "", err
	}

	issRes := gjson.GetBytes(body, "iss")
	if !issRes.Exists() {
		return nil, "", fmt.Errorf("no iss in user info response body")
	}
	iss := issRes.String()

	subRes := gjson.GetBytes(body, "sub")
	if !subRes.Exists() {
		return nil, "", fmt.Errorf("no sub in user info response body")
	}
	sub := subRes.String()

	if iss == "" {
		return nil, "", fmt.Errorf("no iss in user info")
	}

	if sub == "" {
		return nil, "", fmt.Errorf("no sub in user info")
	}

	// This is optional.
	userIDRes := gjson.GetBytes(body, p.UserInfoSuggestedUserIDPath)
	suggestedUserID = userIDRes.String()

	return &UserIdentifier{
		Namespace: uapi.OIDCNamespace,
		Issuer:    iss,
		Subject:   sub,
	}, suggestedUserID, nil
}

type urlTemplate struct {
	base *template.Template
}

func parseURLTemplate(s string) (*urlTemplate, error) {
	t, err := template.New("").Parse(s)
	if err != nil {
		return nil, err
	}
	return &urlTemplate{base: t}, nil
}

func mustParseURLTemplate(s string) *urlTemplate {
	t, err := parseURLTemplate(s)
	if err != nil {
		panic(err)
	}
	return t
}

func (t *urlTemplate) Execute(params interface{}, defaultQuery url.Values) (*url.URL, error) {
	var sb strings.Builder
	err := t.base.Execute(&sb, params)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(sb.String())
	if err != nil {
		return nil, err
	}

	if defaultQuery != nil {
		q := u.Query()
		for k, vs := range defaultQuery {
			if q.Get(k) == "" {
				q[k] = vs
			}
		}
		u.RawQuery = q.Encode()
	}
	return u, nil
}
