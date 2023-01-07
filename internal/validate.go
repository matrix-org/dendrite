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
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
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
	userID := userutil.MakeUserID(localpart, domain)
	return ValidateApplicationServiceUserID(userID)
}

func ValidateApplicationServiceUserID(userID string) error {
	if len(userID) > maxUsernameLength {
		return ErrUsernameTooLong
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil || !validUsernameRegex.MatchString(localpart) {
		return ErrUsernameInvalid
	}

	return nil
}

// userIDIsWithinApplicationServiceNamespace checks to see if a given userID
// falls within any of the namespaces of a given Application Service. If no
// Application Service is given, it will check to see if it matches any
// Application Service's namespace.
func userIDIsWithinApplicationServiceNamespace(
	cfg *config.ClientAPI,
	userID string,
	appservice *config.ApplicationService,
) bool {
	var local, domain, err = gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		// Not a valid userID
		return false
	}

	if !cfg.Matrix.IsLocalServerName(domain) {
		return false
	}

	if appservice != nil {
		if appservice.SenderLocalpart == local {
			return true
		}

		// Loop through given application service's namespaces and see if any match
		for _, namespace := range appservice.NamespaceMap["users"] {
			// AS namespaces are checked for validity in config
			if namespace.RegexpObject.MatchString(userID) {
				return true
			}
		}
		return false
	}

	// Loop through all known application service's namespaces and see if any match
	for _, knownAppService := range cfg.Derived.ApplicationServices {
		if knownAppService.SenderLocalpart == local {
			return true
		}
		for _, namespace := range knownAppService.NamespaceMap["users"] {
			// AS namespaces are checked for validity in config
			if namespace.RegexpObject.MatchString(userID) {
				return true
			}
		}
	}
	return false
}

// usernameMatchesMultipleExclusiveNamespaces will check if a given username matches
// more than one exclusive namespace. More than one is not allowed
func userIDMatchesMultipleExclusiveNamespaces(
	cfg *config.ClientAPI,
	userID string,
) bool {
	// Check namespaces and see if more than one match
	matchCount := 0
	for _, appservice := range cfg.Derived.ApplicationServices {
		if appservice.OwnsNamespaceCoveringUserId(userID) {
			if matchCount++; matchCount > 1 {
				return true
			}
		}
	}
	return false
}

// UsernameMatchesExclusiveNamespaces will check if a given username matches any
// application service's exclusive users namespace
func UsernameMatchesExclusiveNamespaces(
	cfg *config.ClientAPI,
	username string,
) bool {
	userID := userutil.MakeUserID(username, cfg.Matrix.ServerName)
	return cfg.Derived.ExclusiveApplicationServicesUsernameRegexp.MatchString(userID)
}

// validateApplicationService checks if a provided application service token
// corresponds to one that is registered. If so, then it checks if the desired
// username is within that application service's namespace. As long as these
// two requirements are met, no error will be returned.
func ValidateApplicationService(
	cfg *config.ClientAPI,
	username string,
	accessToken string,
) (string, *util.JSONResponse) {
	localpart, domain, err := userutil.ParseUsernameParam(username, cfg.Matrix)
	if err != nil {
		return "", &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
	}

	userID := userutil.MakeUserID(localpart, domain)

	// Check if the token if the application service is valid with one we have
	// registered in the config.
	var matchedApplicationService *config.ApplicationService
	for _, appservice := range cfg.Derived.ApplicationServices {
		if appservice.ASToken == accessToken {
			matchedApplicationService = &appservice
			break
		}
	}
	if matchedApplicationService == nil {
		return "", &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.UnknownToken("Supplied access_token does not match any known application service"),
		}
	}

	// Ensure the desired username is within at least one of the application service's namespaces.
	if !userIDIsWithinApplicationServiceNamespace(cfg, userID, matchedApplicationService) {
		// If we didn't find any matches, return M_EXCLUSIVE
		return "", &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.ASExclusive(fmt.Sprintf(
				"Supplied username %s did not match any namespaces for application service ID: %s", username, matchedApplicationService.ID)),
		}
	}

	// Check this user does not fit multiple application service namespaces
	if userIDMatchesMultipleExclusiveNamespaces(cfg, userID) {
		return "", &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.ASExclusive(fmt.Sprintf(
				"Supplied username %s matches multiple exclusive application service namespaces. Only 1 match allowed", username)),
		}
	}

	// Check username application service is trying to register is valid
	if err := ValidateApplicationServiceUserID(userID); err != nil {
		return "", UsernameResponse(err)
	}

	// No errors, registration valid
	return matchedApplicationService.ID, nil
}
