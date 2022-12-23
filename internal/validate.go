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

package internal

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const (
	maxUsernameLength = 254 // http://matrix.org/speculator/spec/HEAD/intro.html#user-identifiers TODO account for domain

	minPasswordLength = 8   // http://matrix.org/docs/spec/client_server/r0.2.0.html#password-based
	maxPasswordLength = 512 // https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
)

var (
	ErrPasswordTooLong    = fmt.Errorf("password too long: max %d characters", maxPasswordLength)
	ErrPasswordWeak       = fmt.Errorf("password too weak: min %d characters", minPasswordLength)
	ErrUsernameTooLong    = fmt.Errorf("username exceeds the maximum length of %d characters", maxUsernameLength)
	ErrUsernameInvalid    = errors.New("username can only contain characters a-z, 0-9, or '_-./='")
	ErrUsernameUnderscore = errors.New("username cannot start with a '_'")
	validUsernameRegex    = regexp.MustCompile(`^[0-9a-z_\-=./]+$`)
)

// ValidatePassword returns an error if the password is invalid
func ValidatePassword(password string) error {
	// https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	if len(password) > maxPasswordLength {
		return ErrPasswordTooLong
	} else if len(password) > 0 && len(password) < minPasswordLength {
		return ErrPasswordWeak
	}
	return nil
}

// PasswordResponse returns a util.JSONResponse for a given error, if any.
func PasswordResponse(err error) *util.JSONResponse {
	switch err {
	case ErrPasswordWeak:
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.WeakPassword(ErrPasswordWeak.Error()),
		}
	case ErrPasswordTooLong:
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(ErrPasswordTooLong.Error()),
		}
	}
	return nil
}

// ValidateUsername returns an error if the username is invalid
func ValidateUsername(localpart string, domain gomatrixserverlib.ServerName) error {
	// https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	if id := fmt.Sprintf("@%s:%s", localpart, domain); len(id) > maxUsernameLength {
		return ErrUsernameTooLong
	} else if !validUsernameRegex.MatchString(localpart) {
		return ErrUsernameInvalid
	} else if localpart[0] == '_' { // Regex checks its not a zero length string
		return ErrUsernameUnderscore
	}
	return nil
}

// UsernameResponse returns a util.JSONResponse for the given error, if any.
func UsernameResponse(err error) *util.JSONResponse {
	switch err {
	case ErrUsernameTooLong:
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	case ErrUsernameInvalid, ErrUsernameUnderscore:
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
	}
	return nil
}

// ValidateApplicationServiceUsername returns an error if the username is invalid for an application service
func ValidateApplicationServiceUsername(localpart string, domain gomatrixserverlib.ServerName) error {
	if id := fmt.Sprintf("@%s:%s", localpart, domain); len(id) > maxUsernameLength {
		return ErrUsernameTooLong
	} else if !validUsernameRegex.MatchString(localpart) {
		return ErrUsernameInvalid
	}
	return nil
}
