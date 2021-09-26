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
	"github.com/matrix-org/dendrite/setup/config"
)

// GitHubIdentityProvider is a GitHub-flavored identity provider.
var GitHubIdentityProvider IdentityProvider = githubIdentityProvider{
	baseOIDCIdentityProvider: &baseOIDCIdentityProvider{
		AuthURL:                     mustParseURLTemplate("https://github.com/login/oauth/authorize?scope=user:email"),
		AccessTokenURL:              mustParseURLTemplate("https://github.com/login/oauth/access_token"),
		UserInfoURL:                 mustParseURLTemplate("https://api.github.com/user"),
		UserInfoAccept:              "application/vnd.github.v3+json",
		UserInfoEmailPath:           "email",
		UserInfoSuggestedUserIDPath: "login",
	},
}

type githubIdentityProvider struct {
	*baseOIDCIdentityProvider
}

func (githubIdentityProvider) DefaultBrand() string { return config.SSOBrandGitHub }
