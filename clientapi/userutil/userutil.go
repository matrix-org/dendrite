// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package userutil

import (
	"errors"
	"fmt"
	"strings"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// ParseUsernameParam extracts localpart from usernameParam.
// usernameParam can either be a user ID or just the localpart/username.
// If serverName is passed, it is verified against the domain obtained from usernameParam (if present)
// Returns error in case of invalid usernameParam.
func ParseUsernameParam(usernameParam string, cfg *config.Global) (string, spec.ServerName, error) {
	localpart := usernameParam

	if strings.HasPrefix(usernameParam, "@") {
		lp, domain, err := gomatrixserverlib.SplitID('@', usernameParam)

		if err != nil {
			return "", "", errors.New("invalid username")
		}

		if !cfg.IsLocalServerName(domain) {
			return "", "", errors.New("user ID does not belong to this server")
		}

		return lp, domain, nil
	}
	return localpart, cfg.ServerName, nil
}

// MakeUserID generates user ID from localpart & server name
func MakeUserID(localpart string, server spec.ServerName) string {
	return fmt.Sprintf("@%s:%s", localpart, string(server))
}
