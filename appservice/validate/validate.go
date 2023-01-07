// Copyright 2023 The Matrix.org Foundation C.I.C.
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

package validate

import (
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// UserIDIsWithinApplicationServiceNamespace checks to see if a given userID
// falls within any of the namespaces of a given Application Service. If no
// Application Service is given, it will check to see if it matches any
// Application Service's namespace.
func UserIDIsWithinApplicationServiceNamespace(
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

// UsernameMatchesMultipleExclusiveNamespaces will check if a given username matches
// more than one exclusive namespace. More than one is not allowed
func UsernameMatchesMultipleExclusiveNamespaces(
	cfg *config.ClientAPI,
	username string,
) bool {
	userID := userutil.MakeUserID(username, cfg.Matrix.ServerName)

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
// TODO Move somewhere better
func ValidateApplicationService(
	cfg *config.ClientAPI,
	username string,
	accessToken string,
) (string, *util.JSONResponse) {
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

	userID := userutil.MakeUserID(username, cfg.Matrix.ServerName)

	// Ensure the desired username is within at least one of the application service's namespaces.
	if !UserIDIsWithinApplicationServiceNamespace(cfg, userID, matchedApplicationService) {
		// If we didn't find any matches, return M_EXCLUSIVE
		return "", &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.ASExclusive(fmt.Sprintf(
				"Supplied username %s did not match any namespaces for application service ID: %s", username, matchedApplicationService.ID)),
		}
	}

	// Check this user does not fit multiple application service namespaces
	if UsernameMatchesMultipleExclusiveNamespaces(cfg, userID) {
		return "", &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.ASExclusive(fmt.Sprintf(
				"Supplied username %s matches multiple exclusive application service namespaces. Only 1 match allowed", username)),
		}
	}

	// Check username application service is trying to register is valid
	if err := internal.ValidateApplicationServiceUsername(username, cfg.Matrix.ServerName); err != nil {
		return "", internal.UsernameResponse(err)
	}

	// No errors, registration valid
	return matchedApplicationService.ID, nil
}
