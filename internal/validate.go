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

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

const (
	minPasswordLength = 8   // Minimum password length
	maxPasswordLength = 512 // Maximum password length
	maxUsernameLength = 254 // Maximum username length
	sessionIDLength   = 24  // Session ID length
)

var (
	ErrPasswordTooLong    = errors.New("password is too long")
	ErrPasswordWeak       = errors.New("password does not meet the strength requirements")
	ErrUsernameTooLong    = fmt.Errorf("username exceeds the maximum length of %d characters", maxUsernameLength)
	ErrUsernameInvalid    = errors.New("username can only contain characters a-z, 0-9, or '_+-./='")
	ErrUsernameUnderscore = errors.New("username cannot start with a '_'")
	validUsernameRegex    = regexp.MustCompile(`^[0-9a-z_\-+=./]+$`)
)

// PasswordConfig defines the configurable parameters for password validation
type PasswordConfig struct {
	MinLength        int  `yaml:"min_length"`
	MaxLength        int  `yaml:"max_length"`
	RequireUppercase bool `yaml:"require_uppercase"`
	RequireLowercase bool `yaml:"require_lowercase"`
	RequireDigit     bool `yaml:"require_digit"`
	RequireSpecial   bool `yaml:"require_special"`
}

// Default password config
var defaultPasswordConfig = PasswordConfig{
	MinLength:        minPasswordLength,
	MaxLength:        maxPasswordLength,
	RequireUppercase: true,
	RequireLowercase: true,
	RequireDigit:     true,
	RequireSpecial:   true,
}

// ValidatePassword returns an error if the password is invalid according to the config
func ValidatePassword(password string, config PasswordConfig) error {
	if len(password) > config.MaxLength {
		return ErrPasswordTooLong
	} else if len(password) < config.MinLength {
		return ErrPasswordWeak
	}

	var hasUppercase, hasLowercase, hasDigit, hasSpecial bool
	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUppercase = true
		case unicode.IsLower(char):
			hasLowercase = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if config.RequireUppercase && !hasUppercase {
		return ErrPasswordWeak 
	}
	if config.RequireLowercase && !hasLowercase {
		return ErrPasswordWeak
	}
	if config.RequireDigit && !hasDigit {
		return ErrPasswordWeak
	}
	if config.RequireSpecial && !hasSpecial {
		return ErrPasswordWeak
	}

	// Sidecar: Log the password validation attempt (placeholder)
	sidecarLogPasswordValidation(password)

	return nil
}

// Sidecar function to log password validation attempts
func sidecarLogPasswordValidation(password string) {
	// Placeholder for sidecar logging
	fmt.Println("Sidecar log: Password validation attempted")
}

func main() {
	// Example usage
	password := "P@ssw0rd"
	err := ValidatePassword(password, defaultPasswordConfig)
	if err != nil {
		fmt.Println("Password validation failed:", err)
	} else {
		fmt.Println("Password is valid")
	}
}

// ValidateUsername returns an error if the username is invalid
func ValidateUsername(localpart string, domain spec.ServerName) error {
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
			JSON: spec.BadJSON(err.Error()),
		}
	case ErrUsernameInvalid, ErrUsernameUnderscore:
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidUsername(err.Error()),
		}
	}
	return nil
}

// ValidateApplicationServiceUsername returns an error if the username is invalid for an application service
func ValidateApplicationServiceUsername(localpart string, domain spec.ServerName) error {
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
	var localpart, domain, err = gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		// Not a valid userID
		return false
	}

	if !cfg.Matrix.IsLocalServerName(domain) {
		// This is a federated userID
		return false
	}

	if localpart == appservice.SenderLocalpart {
		// This is the application service bot userID
		return true
	}

	// Loop through given application service's namespaces and see if any match
	for _, namespace := range appservice.NamespaceMap["users"] {
		// Application service namespaces are checked for validity in config
		if namespace.RegexpObject.MatchString(userID) {
			return true
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

// ValidateApplicationServiceRequest checks if a provided application service
// token corresponds to one that is registered, and, if so, checks if the
// supplied userIDOrLocalpart is within that application service's namespace.
//
// As long as these two requirements are met, the matched application service
// ID will be returned. Otherwise, it will return a JSON response with the
// appropriate error message.
func ValidateApplicationServiceRequest(
	cfg *config.ClientAPI,
	userIDOrLocalpart string,
	accessToken string,
) (string, *util.JSONResponse) {
	localpart, domain, err := userutil.ParseUsernameParam(userIDOrLocalpart, cfg.Matrix)
	if err != nil {
		return "", &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: spec.InvalidUsername(err.Error()),
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
			JSON: spec.UnknownToken("Supplied access_token does not match any known application service"),
		}
	}

	// Ensure the desired username is within at least one of the application service's namespaces.
	if !userIDIsWithinApplicationServiceNamespace(cfg, userID, matchedApplicationService) {
		// If we didn't find any matches, return M_EXCLUSIVE
		return "", &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.ASExclusive(fmt.Sprintf(
				"Supplied username %s did not match any namespaces for application service ID: %s", userIDOrLocalpart, matchedApplicationService.ID)),
		}
	}

	// Check this user does not fit multiple application service namespaces
	if userIDMatchesMultipleExclusiveNamespaces(cfg, userID) {
		return "", &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.ASExclusive(fmt.Sprintf(
				"Supplied username %s matches multiple exclusive application service namespaces. Only 1 match allowed", userIDOrLocalpart)),
		}
	}

	// Check username application service is trying to register is valid
	if err := ValidateApplicationServiceUserID(userID); err != nil {
		return "", UsernameResponse(err)
	}

	// No errors, registration valid
	return matchedApplicationService.ID, nil
}
