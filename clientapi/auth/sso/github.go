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
	"net/http"

	"github.com/matrix-org/dendrite/setup/config"
)

func newGitHubIdentityProvider(cfg *config.IdentityProvider, hc *http.Client) identityProvider {
	return &oauth2IdentityProvider{
		cfg:       cfg,
		oauth2Cfg: &cfg.OAuth2,
		hc:        hc,

		authorizationURL: "https://github.com/login/oauth/authorize",
		accessTokenURL:   "https://github.com/login/oauth/access_token",
		userInfoURL:      "https://api.github.com/user",

		scopes:              []string{"user:email"},
		responseMimeType:    "application/vnd.github.v3+json",
		subPath:             "id",
		emailPath:           "email",
		displayNamePath:     "name",
		suggestedUserIDPath: "login",
	}
}
