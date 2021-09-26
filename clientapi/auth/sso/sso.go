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
	"net/url"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
)

type IdentityProvider interface {
	DefaultBrand() string

	AuthorizationURL(context.Context, *IdentityProviderRequest) (string, error)
	ProcessCallback(context.Context, *IdentityProviderRequest, url.Values) (*CallbackResult, error)
}

type IdentityProviderRequest struct {
	System        *config.IdentityProvider
	CallbackURL   string
	DendriteNonce string
}

type CallbackResult struct {
	RedirectURL     string
	Identifier      *userutil.ThirdPartyIdentifier
	SuggestedUserID string
}

type IdentityProviderType string

const (
	TypeGitHub IdentityProviderType = config.SSOBrandGitHub
)

func GetIdentityProvider(t IdentityProviderType) IdentityProvider {
	switch t {
	case TypeGitHub:
		return GitHubIdentityProvider
	default:
		return nil
	}
}
