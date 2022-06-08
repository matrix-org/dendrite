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
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/matrix-org/dendrite/setup/config"
	uapi "github.com/matrix-org/dendrite/userapi/api"
)

// maxHTTPTimeout is an upper bound on an HTTP request to an SSO
// backend. The individual request context deadlines are also
// honoured.
const maxHTTPTimeout = 10 * time.Second

// An Authenticator keeps a set of identity providers and dispatches
// calls to one of them, based on configured ID.
type Authenticator struct {
	providers map[string]identityProvider
}

func NewAuthenticator(cfg *config.SSO) (*Authenticator, error) {
	hc := &http.Client{
		Timeout: maxHTTPTimeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			Proxy:             http.ProxyFromEnvironment,
		},
	}

	a := &Authenticator{
		providers: make(map[string]identityProvider, len(cfg.Providers)),
	}
	for _, pcfg := range cfg.Providers {
		pcfg = pcfg.WithDefaults()

		switch pcfg.Type {
		case config.SSOTypeOIDC:
			a.providers[pcfg.ID] = newOIDCIdentityProvider(&pcfg, hc)
		case config.SSOTypeGitHub:
			a.providers[pcfg.ID] = newGitHubIdentityProvider(&pcfg, hc)
		default:
			return nil, fmt.Errorf("unknown SSO provider type: %s", pcfg.Type)
		}
	}

	return a, nil
}

func (auth *Authenticator) AuthorizationURL(ctx context.Context, providerID, callbackURL, nonce string) (string, error) {
	p := auth.providers[providerID]
	if p == nil {
		return "", fmt.Errorf("unknown identity provider: %s", providerID)
	}
	return p.AuthorizationURL(ctx, callbackURL, nonce)
}

func (auth *Authenticator) ProcessCallback(ctx context.Context, providerID, callbackURL, nonce string, query url.Values) (*CallbackResult, error) {
	p := auth.providers[providerID]
	if p == nil {
		return nil, fmt.Errorf("unknown identity provider: %s", providerID)
	}
	return p.ProcessCallback(ctx, callbackURL, nonce, query)
}

type identityProvider interface {
	AuthorizationURL(ctx context.Context, callbackURL, nonce string) (string, error)
	ProcessCallback(ctx context.Context, callbackURL, nonce string, query url.Values) (*CallbackResult, error)
}

type CallbackResult struct {
	RedirectURL     string
	Identifier      *UserIdentifier
	DisplayName     string
	SuggestedUserID string
}

type UserIdentifier struct {
	Namespace       uapi.SSOIssuerNamespace
	Issuer, Subject string
}
