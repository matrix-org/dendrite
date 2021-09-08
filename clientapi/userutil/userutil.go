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

package userutil

import (
	"errors"
	"fmt"
	"strings"

	"github.com/matrix-org/gomatrixserverlib"
)

// ParseUsernameParam extracts localpart from usernameParam.
// usernameParam can either be a user ID or just the localpart/username.
// If serverName is passed, it is verified against the domain obtained from usernameParam (if present)
// Returns error in case of invalid usernameParam.
func ParseUsernameParam(usernameParam string, expectedServerName *gomatrixserverlib.ServerName) (string, error) {
	localpart := usernameParam

	if strings.HasPrefix(usernameParam, "@") {
		lp, domain, err := gomatrixserverlib.SplitID('@', usernameParam)

		if err != nil {
			return "", errors.New("invalid username")
		}

		if expectedServerName != nil && domain != *expectedServerName {
			return "", errors.New("user ID does not belong to this server")
		}

		localpart = lp
	}
	return localpart, nil
}

// MakeUserID generates user ID from localpart & server name
func MakeUserID(localpart string, server gomatrixserverlib.ServerName) string {
	return fmt.Sprintf("@%s:%s", localpart, string(server))
}
