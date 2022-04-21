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
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	userApi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/tidwall/gjson"
)

type LoginPublicKeyEthereum struct {
	// Todo: See https://...
	Type          string                      `json:"type"`
	Address       string                      `json:"address"`
	Session       string                      `json:"session"`
	Message       string                      `json:"message"`
	Signature     string                      `json:"signature"`
	HashFields    publicKeyEthereumHashFields `json:"hashFields"`
	HashFieldsRaw string                      // Raw base64 encoded string of MessageFields for hash verification

	userAPI userApi.UserRegisterAPI
	config  *config.ClientAPI
}

type publicKeyEthereumHashFields struct {
	// Todo: See https://...
	Domain  string `json:"domain"`  // home server domain
	Address string `json:"address"` // Ethereum address. 0x...
	Nonce   string `json:"nonce"`   // session ID
	Version string `json:"version"` // version of the Matrix public key spec that the client is complying with
	ChainId string `json:"chainId"` // blockchain network ID.
}

type publicKeyEthereumRequiredFields struct {
	From string // Sender
	To   string // Recipient
	Hash string // Hash of JSON representation of the message fields
}

func CreatePublicKeyEthereumHandler(
	reqBytes []byte,
	userAPI userApi.UserRegisterAPI,
	config *config.ClientAPI,
) (*LoginPublicKeyEthereum, *jsonerror.MatrixError) {
	var pk LoginPublicKeyEthereum
	if err := json.Unmarshal(reqBytes, &pk); err != nil {
		return nil, jsonerror.BadJSON("auth")
	}

	hashFields := gjson.GetBytes(reqBytes, "hashFields")
	if !hashFields.Exists() {
		return nil, jsonerror.BadJSON("auth.hashFields")
	}

	pk.config = config
	pk.userAPI = userAPI
	// Save raw bytes for hash verification later.
	pk.HashFieldsRaw = hashFields.Raw
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

	res := userApi.QueryAccountAvailabilityResponse{}
	if err := pk.userAPI.QueryAccountAvailability(ctx, &userApi.QueryAccountAvailabilityRequest{
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
	// Check signature to verify message was not tempered
	isVerified := verifySignature(pk.Address, []byte(pk.Message), pk.Signature)
	if !isVerified {
		return false, jsonerror.InvalidSignature("")
	}

	// Extract the required message fields for validation
	requiredFields, err := extractRequiredMessageFields(pk.Message)
	if err != nil {
		return false, jsonerror.MissingParam("message does not contain domain, address, or hash")
	}

	// Verify that the hash is valid for the message fields.
	if !verifyHash(pk.HashFieldsRaw, requiredFields.Hash) {
		return false, jsonerror.InvalidParam("error verifying message hash")
	}

	// Unmarshal the hashFields for further validation
	var authData publicKeyEthereumHashFields
	if err := json.Unmarshal([]byte(pk.HashFieldsRaw), &authData); err != nil {
		return false, jsonerror.BadJSON("auth.hashFields")
	}

	// Error if the message is not from the expected public address
	if pk.Address != requiredFields.From || requiredFields.From != pk.HashFields.Address {
		return false, jsonerror.InvalidParam("address")
	}

	// Error if the message is not for the home server
	if requiredFields.To != pk.HashFields.Domain {
		return false, jsonerror.InvalidParam("domain")
	}

	// No errors.
	return true, nil
}

func (pk LoginPublicKeyEthereum) CreateLogin() *Login {
	identifier := LoginIdentifier{
		Type: "m.id.user",
		User: pk.Address,
	}
	login := Login{
		Identifier: identifier,
	}
	return &login
}

// The required fields in the signed message are:
// 1. Domain -- home server. First non-whitespace characters in the first line.
// 2. Address -- public address of the user. Starts with 0x... in the second line on its own.
// 3. Hash -- Base64-encoded hash string of the metadata that represents the message.
// The rest of the fields are informational, and will be used in the future.
var regexpAuthority = regexp.MustCompile(`^\S+`)
var regexpAddress = regexp.MustCompile(`\n(?P<address>0x\w+)\n`)
var regexpHash = regexp.MustCompile(`\nHash: (?P<hash>.*)\n`)

func extractRequiredMessageFields(message string) (*publicKeyEthereumRequiredFields, error) {
	var requiredFields publicKeyEthereumRequiredFields
	/*
		service.org wants you to sign in with your account:
		0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2

		I accept the ServiceOrg Terms of Service: https://service.org/tos

		Hash: yfSIwarByPfKFxeYSCWN3XoIgNgeEFJffbwFA+JxYbA=
	*/

	requiredFields.To = regexpAuthority.FindString(message)

	from := regexpAddress.FindStringSubmatch(message)
	if len(from) == 2 {
		requiredFields.From = from[1]
	}

	hash := regexpHash.FindStringSubmatch(message)
	if len(hash) == 2 {
		requiredFields.Hash = hash[1]
	}

	if len(requiredFields.To) == 0 || len(requiredFields.From) == 0 || len(requiredFields.Hash) == 0 {
		return nil, errors.New("required message fields are missing")
	}

	// Make these fields case-insensitive
	requiredFields.From = strings.ToLower(requiredFields.From)
	requiredFields.To = strings.ToLower(requiredFields.To)

	return &requiredFields, nil
}

func verifySignature(from string, message []byte, signature string) bool {
	decodedSig := hexutil.MustDecode(signature)

	message = accounts.TextHash(message)
	// Issue: https://stackoverflow.com/questions/49085737/geth-ecrecover-invalid-signature-recovery-id
	// Fix: https://gist.github.com/dcb9/385631846097e1f59e3cba3b1d42f3ed#file-eth_sign_verify-go
	decodedSig[crypto.RecoveryIDOffset] -= 27 // Transform yellow paper V from 27/28 to 0/1

	recovered, err := crypto.SigToPub(message, decodedSig)
	if err != nil {
		return false
	}

	recoveredAddr := crypto.PubkeyToAddress(*recovered)

	addressStr := strings.ToLower(recoveredAddr.Hex())
	return from == addressStr
}

func verifyHash(rawStr string, expectedHash string) bool {
	hash := crypto.Keccak256([]byte(rawStr))
	hashStr := base64.StdEncoding.EncodeToString(hash)
	return expectedHash == hashStr
}
