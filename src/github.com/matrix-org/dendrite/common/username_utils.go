// Copyright 2018 New Vector Ltd
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

package common

import (
	"errors"
	"strings"

	"github.com/matrix-org/gomatrixserverlib"
)

// GetLocalpartDomainFromUserID extracts localpart & domain of server from userID.
// Returns error in case of invalid username.
func GetLocalpartDomainFromUserID(userID string,
) (string, gomatrixserverlib.ServerName, error) {
	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)

	if err != nil {
		return localpart, domain, err
	}

	return localpart, domain, nil
}

// GetLocalpartFromUsername extracts localpart from userID
// userID can either be a user ID or just the localpart.
// Returns error in case of invalid username.
func GetLocalpartFromUsername(userID string,
) (string, error) {
	localpart := userID

	if strings.HasPrefix(userID, "@") {
		lp, domain, err := GetLocalpartDomainFromUserid(userID)

		if err != nil {
			return "", errors.New("Invalid username")
		}

		if domain != cfg.Matrix.ServerName {
			return "", errors.New("User ID not ours")
		}
		localpart = lp
	}
	return localpart, nil
}
