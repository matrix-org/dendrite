// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// This file handles all the m.login.token logic

// GetAccountByLocalpart function implemented by the appropriate database type
type GetAccountByLocalpart func(ctx context.Context, localpart string) (*api.Account, error)

// LoginTokenRequest struct to hold the possible parameters from an m.login.token http request
type LoginTokenRequest struct {
	Login
	Token string `json:"token"`
	TxnID string `json:"txn_id"`
}

// LoginTypeToken holds the configs and the appropriate GetAccountByLocalpart function for the database
type LoginTypeToken struct {
	GetAccountByLocalpart GetAccountByLocalpart
	Config                *config.ClientAPI
}

// Name returns the expected type of "m.login.token"
func (t *LoginTypeToken) Name() string {
	return "m.login.token"
}

// Request returns a struct of type LoginTokenRequest
func (t *LoginTypeToken) Request() interface{} {
	return &LoginTokenRequest{}
}

// Type of the LoginToken
type loginToken struct {
	UserID       string
	CreationTime int64
	RandomPart   string
}

// Login completes the whole token validation, user verification for m.login.token
// returns a struct of type *auth.Login which has the users details
func (t *LoginTypeToken) Login(ctx context.Context, req interface{}) (*Login, *util.JSONResponse) {
	r := req.(*LoginTokenRequest)
	userID, err := validateLoginToken(r.Token, r.TxnID, &t.Config.Matrix.ServerName)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidArgumentValue(err.Error()),
		}
	}
	r.Login.Identifier.User = userID
	r.Login.Identifier.Type = "m.id.user"

	return &r.Login, nil
}

// Decodes and validates a LoginToken
// Accepts the base64 encoded token string as param
// Checks the time expiry, userID (only the format, doesn't check to see if the user exists)
// Also checks the DB to see if the token exists
// Returns the localpart if successful
func validateLoginToken(tokenStr string, txnID string, serverName *gomatrixserverlib.ServerName) (string, error) {
	token, err := decodeLoginToken(tokenStr)
	if err != nil {
		return "", err
	}

	// check whether the token has a valid time.
	// TODO: should this 5 second window be configurable?
	if time.Now().Unix()-token.CreationTime > 5 {
		return "", errors.New("Token has expired")
	}

	// check whether the UserID is malformed
	if !strings.Contains(token.UserID, "@") {
		// TODO: should we reveal details about the error with the token or give vague responses instead?
		return "", errors.New("Invalid UserID")
	}
	if _, err := userutil.ParseUsernameParam(token.UserID, serverName); err != nil {
		return "", err
	}

	// check in the database
	if err := checkDBToken(tokenStr, txnID); err != nil {
		return "", err
	}

	return token.UserID, nil
}

// GenerateLoginToken creates a login token which is a base64 encoded string of (userID+time+random)
// returns an error if it cannot create a random string
func GenerateLoginToken(userID string) (string, error) {
	// the time of token creation
	timePart := []byte(strconv.FormatInt(time.Now().Unix(), 10))

	// the random part of the token
	randPart := make([]byte, 10)
	if _, err := rand.Read(randPart); err != nil {
		return "", err
	}

	// url-safe no padding
	return base64.RawURLEncoding.EncodeToString([]byte(userID)) + "." + base64.RawURLEncoding.EncodeToString(timePart) + "." + base64.RawURLEncoding.EncodeToString(randPart), nil
}

// Decodes the given tokenStr into a LoginToken struct
func decodeLoginToken(tokenStr string) (*loginToken, error) {
	// split the string into it's constituent parts
	strParts := strings.Split(tokenStr, ".")
	if len(strParts) != 3 {
		return nil, errors.New("Malformed token string")
	}

	var token loginToken
	// decode each of the strParts
	userBytes, err := base64.RawURLEncoding.DecodeString(strParts[0])
	if err != nil {
		return nil, errors.New("Invalid user ID")
	}
	token.UserID = string(userBytes)

	// first decode the time to a string
	timeBytes, err := base64.RawURLEncoding.DecodeString(strParts[1])
	if err != nil {
		return nil, errors.New("Invalid creation time")
	}
	// now convert the string to an integer
	creationTime, err := strconv.ParseInt(string(timeBytes), 10, 64)
	if err != nil {
		return nil, errors.New("Invalid creation time")
	}
	token.CreationTime = creationTime

	randomBytes, err := base64.RawURLEncoding.DecodeString(strParts[2])
	if err != nil {
		return nil, errors.New("Invalid random part")
	}
	token.UserID = string(randomBytes)

	token = loginToken{
		UserID:       string(userBytes),
		CreationTime: creationTime,
		RandomPart:   string(randomBytes),
	}
	return &token, nil
}

// Checks whether the token exists in the DB and whether the token is assigned to the current transaction ID
// Does not validate the userID or the creation time expiry
// Returns nil if successful
func checkDBToken(tokenStr string, txnID string) error {
	// if the client has provided a transaction id, try to lock the token to that ID
	if txnID != "" {
		if err := LinkToken(tokenStr, txnID); err != nil {
			// TODO: should we abort the login attempt or something else?
		}
	}
	return nil
}

// StoreLoginToken stores the login token in the database
// Returns nil if successful
func StoreLoginToken(tokenStr string) error {
	return nil
}

// DeleteLoginToken Deletes a token from the DB
// used to delete a token that has already been used
// Returns nil if successful
func DeleteLoginToken(tokenStr string) error {
	return nil
}

// LinkToken Links a token to a transaction ID so no other client can try to login using that token
// as specified in https://matrix.org/docs/spec/client_server/r0.6.1#token-based
func LinkToken(tokenStr string, txnID string) error {
	return nil
}
