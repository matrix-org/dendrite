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
	"encoding/json"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/spruceid/siwe-go"
)

type LoginPublicKeyEthereum struct {
	// https://github.com/tak-hntlabs/matrix-spec-proposals/blob/main/proposals/3782-matrix-publickey-login-spec.md#client-sends-login-request-with-authentication-data
	Type      string `json:"type"`
	Address   string `json:"address"`
	Session   string `json:"session"`
	Message   string `json:"message"`
	Signature string `json:"signature"`

	userAPI userapi.ClientUserAPI
	config  *config.ClientAPI
}

func CreatePublicKeyEthereumHandler(
	reqBytes []byte,
	userAPI userapi.ClientUserAPI,
	config *config.ClientAPI,
) (*LoginPublicKeyEthereum, *jsonerror.MatrixError) {
	var pk LoginPublicKeyEthereum
	if err := json.Unmarshal(reqBytes, &pk); err != nil {
		return nil, jsonerror.BadJSON("auth")
	}

	pk.config = config
	pk.userAPI = userAPI
	// Case-insensitive
	pk.Address = strings.ToLower(pk.Address)

	return &pk, nil
}

func (pk LoginPublicKeyEthereum) GetSession() string {
	return pk.Session
}

func (pk LoginPublicKeyEthereum) GetType() string {
	return pk.Type
}

func (pk LoginPublicKeyEthereum) AccountExists(ctx context.Context) (string, *jsonerror.MatrixError) {
	localPart, err := userutil.ParseUsernameParam(pk.Address, &pk.config.Matrix.ServerName)
	if err != nil {
		// userId does not exist
		return "", jsonerror.Forbidden("the address is incorrect, or the account does not exist.")
	}

	res := userapi.QueryAccountAvailabilityResponse{}
	if err := pk.userAPI.QueryAccountAvailability(ctx, &userapi.QueryAccountAvailabilityRequest{
		Localpart: localPart,
	}, &res); err != nil {
		return "", jsonerror.Unknown("failed to check availability: " + err.Error())
	}

	if res.Available {
		return "", jsonerror.Forbidden("the address is incorrect, account does not exist")
	}

	return localPart, nil
}

func (pk LoginPublicKeyEthereum) ValidateLoginResponse() (bool, *jsonerror.MatrixError) {
	// Parse the message to extract all the fields.
	message, err := siwe.ParseMessage(pk.Message)
	if err != nil {
		return false, jsonerror.InvalidParam("auth.message")
	}

	// Check signature to verify message was not tempered
	_, err = message.Verify(pk.Signature, (*string)(&pk.config.Matrix.ServerName), nil, nil)
	if err != nil {
		return false, jsonerror.InvalidSignature(err.Error())
	}

	// Error if the chainId is not supported by the server.
	if !contains(pk.config.PublicKeyAuthentication.Ethereum.ChainIDs, message.GetChainID()) {
		return false, jsonerror.Forbidden("chainId")
	}

	// No errors.
	return true, nil
}

func (pk LoginPublicKeyEthereum) CreateLogin() *Login {
	identifier := LoginIdentifier{
		Type: "m.id.publickey",
		User: pk.Address,
	}
	login := Login{
		Identifier: identifier,
	}
	return &login
}

func contains(list []int, element int) bool {
	for _, i := range list {
		if i == element {
			return true
		}
	}
	return false
}
