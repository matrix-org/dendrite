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

package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/stretchr/testify/assert"
)

func TestLoginPublicKeyNewSession(t *testing.T) {
	// Setup
	var userAPI fakePublicKeyUserApi
	ctx := context.Background()
	loginContext := createLoginContext(t)

	test := struct {
		Body string
	}{
		Body: `{ "type": "m.login.publickey" }`,
	}

	// Test
	login, cleanup, err := LoginFromJSONReader(
		ctx,
		strings.NewReader(test.Body),
		&userAPI,
		&userAPI,
		&userAPI,
		loginContext.userInteractive,
		loginContext.config)

	if cleanup != nil {
		cleanup(ctx, nil)
	}

	// Asserts
	assert := assert.New(t)
	assert.NotNilf(
		err,
		"err actual: not nil returned %+v, expected: nil", login)
	assert.Truef(
		err.Code == http.StatusUnauthorized,
		"err.Code actual: %v, expected: %v", err.Code, http.StatusUnauthorized)
	challenge := err.JSON.(Challenge)
	assert.NotEmptyf(challenge.Session, "challenge.Session")
	assert.Truef(
		authtypes.LoginTypePublicKeyEthereum == challenge.Flows[0].Stages[0],
		"challenge.Flows[0].Stages[0] actual: %v, expected: %v", challenge.Flows[0].Stages[0], authtypes.LoginTypePublicKeyEthereum)
	params := challenge.Params[authtypes.LoginTypePublicKeyEthereum]
	assert.NotEmptyf(
		params,
		"challenge.Params[\"%v\"] actual %v, expected %v",
		authtypes.LoginTypePublicKeyEthereum,
		params,
		"[object]")
	ethParams := params.(config.EthereumAuthParams)
	assert.NotEmptyf(ethParams.ChainIDs, "ChainIDs actual: empty, expected not empty")
	assert.NotEmptyf(ethParams.Version, "Version actual: \"\", expected: not empty")
}

func TestLoginPublicKeyInvalidSessionId(t *testing.T) {
	// Setup
	var userAPI fakePublicKeyUserApi
	ctx := context.Background()
	loginContext := createLoginContext(t)

	test := struct {
		Body string
	}{
		Body: `{
			"type": "m.login.publickey",
			"auth": {
				"type": "m.login.publickey.ethereum",
				"session": "invalid_session_id"
			}
		 }`,
	}

	// Test
	_, cleanup, err := LoginFromJSONReader(
		ctx,
		strings.NewReader(test.Body),
		&userAPI,
		&userAPI,
		&userAPI,
		loginContext.userInteractive,
		loginContext.config)

	if cleanup != nil {
		cleanup(ctx, nil)
	}

	// Asserts
	assert := assert.New(t)
	assert.Truef(
		err.Code == http.StatusUnauthorized,
		"err.Code actual %v, expected %v", err.Code, http.StatusUnauthorized)
}

func TestLoginPublicKeyInvalidAuthType(t *testing.T) {
	// Setup
	var userAPI fakePublicKeyUserApi
	ctx := context.Background()
	loginContext := createLoginContext(t)

	test := struct {
		Body string
	}{
		Body: `{
			"type": "m.login.publickey",
			"auth": {
				"type": "m.login.publickey.someAlgo"
			}
		 }`,
	}

	// Test
	_, cleanup, err := LoginFromJSONReader(
		ctx,
		strings.NewReader(test.Body),
		&userAPI,
		&userAPI,
		&userAPI,
		loginContext.userInteractive,
		loginContext.config)

	if cleanup != nil {
		cleanup(ctx, nil)
	}

	// Asserts
	assert := assert.New(t)
	assert.NotNil(err, "Expected an err response.actual: nil")
	assert.Truef(
		err.Code == http.StatusUnauthorized,
		"err.Code actual %v, expected %v", err.Code, http.StatusUnauthorized)
	_, ok := err.JSON.(Challenge)
	assert.False(
		ok,
		"should not return a Challenge response")
}
