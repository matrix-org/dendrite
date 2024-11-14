// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package userutil

import (
	"testing"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

var (
	localpart                         = "somelocalpart"
	serverName        spec.ServerName = "someservername"
	invalidServerName spec.ServerName = "invalidservername"
	goodUserID                        = "@" + localpart + ":" + string(serverName)
	badUserID                         = "@bad:user:name@noservername:"
)

// TestGoodUserID checks that correct localpart is returned for a valid user ID.
func TestGoodUserID(t *testing.T) {
	cfg := &config.Global{
		SigningIdentity: fclient.SigningIdentity{
			ServerName: serverName,
		},
	}

	lp, _, err := ParseUsernameParam(goodUserID, cfg)

	if err != nil {
		t.Error("User ID Parsing failed for ", goodUserID, " with error: ", err.Error())
	}

	if lp != localpart {
		t.Error("Incorrect username, returned: ", lp, " should be: ", localpart)
	}
}

// TestWithLocalpartOnly checks that localpart is returned when usernameParam contains only localpart.
func TestWithLocalpartOnly(t *testing.T) {
	cfg := &config.Global{
		SigningIdentity: fclient.SigningIdentity{
			ServerName: serverName,
		},
	}

	lp, _, err := ParseUsernameParam(localpart, cfg)

	if err != nil {
		t.Error("User ID Parsing failed for ", localpart, " with error: ", err.Error())
	}

	if lp != localpart {
		t.Error("Incorrect username, returned: ", lp, " should be: ", localpart)
	}
}

// TestIncorrectDomain checks for error when there's server name mismatch.
func TestIncorrectDomain(t *testing.T) {
	cfg := &config.Global{
		SigningIdentity: fclient.SigningIdentity{
			ServerName: invalidServerName,
		},
	}

	_, _, err := ParseUsernameParam(goodUserID, cfg)

	if err == nil {
		t.Error("Invalid Domain should return an error")
	}
}

// TestBadUserID checks that ParseUsernameParam fails for invalid user ID
func TestBadUserID(t *testing.T) {
	cfg := &config.Global{
		SigningIdentity: fclient.SigningIdentity{
			ServerName: serverName,
		},
	}

	_, _, err := ParseUsernameParam(badUserID, cfg)

	if err == nil {
		t.Error("Illegal User ID should return an error")
	}
}
