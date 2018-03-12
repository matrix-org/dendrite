// Copyright 2018 Vector Creations Ltd
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

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

// GetUserFromToken returns the user associated with the token
// Returns the error if something goes wrong.
// Warning: Does not validate the token. Use ValidateToken for that.
func GetUserFromToken(token string) (user string, err error) {
	mac, err := deserializeMacaroon(token)

	if err != nil {
		return
	}
	user = string(mac.Id()[:])
	return
}

// ValidateToken validates that the Token is understood and was signed by this server.
// Returns nil if token is valid, otherwise returns a error response which can be sent to client.
func ValidateToken(op TokenOptions, token string) *util.JSONResponse {
	macaroon, err := deserializeMacaroon(token)

	if err != nil {
		return &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.UnknownToken("Token does not represent a valid macaroon"),
		}
	}

	caveats, err := macaroon.VerifySignature(op.ServerMacaroonSecret, nil)

	if err != nil {
		return &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.UnknownToken("Provided token was not issued by this server"),
		}
	}

	err = verifyCaveats(caveats, op.UserID)

	if err != nil {
		return &util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("Provided token not authorized"),
		}
	}
	return nil
}

// verifyCaveats verifies caveats associated with a login token macaroon.
// which are "gen = 1", "user_id = ...", "time < ..."
// Returns nil on successful verification, else returns the error.
func verifyCaveats(caveats []string, userID string) error {
	// let last 4 bit be Uabc where,
	// U: unknownCaveat
	// a, b, c: 1st, 2nd & 3rd caveat to be verified respectively.
	var verified uint8
	now := time.Now().Second()

	for _, caveat := range caveats {
		switch {
		case caveat == Gen:
			verified |= 1
		case strings.HasPrefix(caveat, UserPrefix):
			if caveat[len(UserPrefix):] == userID {
				verified |= 2
			}
		case strings.HasPrefix(caveat, TimePrefix):
			if verifyExpiry(caveat[len(TimePrefix):], now) {
				verified |= 4
			}
		default:
			verified |= 8
			break
		}
	}

	// Check that all three caveats are verified and no extra caveats
	// i.e. Uabc == 0111
	if verified == 7 {
		return nil
	} else if verified >= 8 {
		return errors.New("Unknown caveat present")
	}

	return errors.New("Required caveats not present")
}

func verifyExpiry(t string, now int) bool {
	expiry, err := strconv.Atoi(t)

	if err != nil {
		return false
	}
	return now < expiry
}
