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

package tokens

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// GetUserFromToken returns the user associated with the token
// Returns the error if something goes wrong.
// Warning: Does not validate the token. Use ValidateToken for that.
func GetUserFromToken(token string) (user string, err error) {
	mac, err := DeSerializeMacaroon(token)

	if err != nil {
		return
	}

	// ID() returns a []byte, so convert it to string
	user = string(mac.Id()[:])
	return
}

// ValidateToken validates that the Token is understood and was signed by this server.
// Returns nil if token is valid, otherwise returns an error/
func ValidateToken(op TokenOptions, token string) error {
	macaroon, err := DeSerializeMacaroon(token)

	if err != nil {
		return errors.New("Token does not represent a valid macaroon")
	}

	// VerifySignature returns all caveats in the macaroon
	caveats, err := macaroon.VerifySignature(op.ServerMacaroonSecret, nil)

	if err != nil {
		return errors.New("Provided token was not issued by this server")
	}

	err = verifyCaveats(caveats, op.UserID)

	if err != nil {
		return errors.New("Provided token not authorized")
	}
	return nil
}

// verifyCaveats verifies caveats associated with a login macaroon token.
// Which are "gen = 1", "user_id = ...", "time < ..."
// Returns nil on successful verification, else returns an error.
func verifyCaveats(caveats []string, userID string) error {
	// verified is a bitmask
	// let last 4 bit be uabc where,
	// u denotes any unknownCaveat
	// a, b, c denotes the three caveats to be verified respectively.
	var verified uint8
	now := time.Now().Second()

	for _, caveat := range caveats {
		switch {
		case caveat == Gen:
			// "gen = 1"
			verified |= 1
		case strings.HasPrefix(caveat, UserPrefix):
			// "user_id = ..."
			if caveat[len(UserPrefix):] == userID {
				verified |= 2
			}
		case strings.HasPrefix(caveat, TimePrefix):
			// "time < ..."
			if verifyExpiry(caveat[len(TimePrefix):], now) {
				verified |= 4
			}
		default:
			// Unknown caveat
			verified |= 8
		}
	}

	// Check that all three caveats are verified and no extra caveats are present
	// i.e. uabc == 0111, which implies verified == 7
	if verified != 7 {
		if verified >= 8 {
			return errors.New("Unknown caveat present")
		}

		return errors.New("Required caveats not present")
	}

	return nil
}

// verifyExpiry verifies an expiry caveat
func verifyExpiry(t string, now int) bool {
	expiry, err := strconv.Atoi(t)

	if err != nil {
		return false
	}
	return now < expiry
}
