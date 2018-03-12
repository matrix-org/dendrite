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
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/go-macaroon/macaroon"
)

const (
	macaroonVersion = macaroon.V2
	defaultDuration = 2 * 60
	// UserPrefix for user_id caveat
	UserPrefix = "user_id = "
	// TimePrefix for expiry caveat
	TimePrefix = "time < "
	// Gen represents an unique identifier used as a caveat
	Gen = "gen = 1"
)

// TokenOptions represent parameters of Token
type TokenOptions struct {
	ServerMacaroonSecret []byte `yaml:"private_key"`
	ServerName           string `yaml:"server_name"`
	UserID               string `json:"user_id"`
	Duration             int    // optional
}

// GenerateLoginToken generates a short term login token to be used as
// token authentication ("m.login.token")
func GenerateLoginToken(op TokenOptions) (string, error) {
	mac, err := generateBaseMacaroon(op.ServerMacaroonSecret, op.ServerName, op.UserID)

	if err != nil {
		return "", err
	}

	if op.Duration == 0 {
		op.Duration = defaultDuration
	}
	now := time.Now().Second()
	expiryCaveat := TimePrefix + strconv.Itoa(now+op.Duration)
	err = mac.AddFirstPartyCaveat([]byte(expiryCaveat))

	if err != nil {
		return "", macaroonError(err)
	}

	urlSafeEncode, err := serializeMacaroon(*mac)
	return urlSafeEncode, macaroonError(err)
}

// generateBaseMacaroon generates a base macaroon common for accessToken & loginToken
// Returns a macaroon tied with userID,
// returns formated error if something goes wrong.
func generateBaseMacaroon(
	secret []byte, ServerName string, userID string,
) (*macaroon.Macaroon, error) {
	mac, err := macaroon.New(secret, []byte(userID), ServerName, macaroonVersion)
	if err != nil {
		return nil, macaroonError(err)
	}

	err = mac.AddFirstPartyCaveat([]byte(Gen))
	if err != nil {
		return nil, macaroonError(err)
	}

	err = mac.AddFirstPartyCaveat([]byte(UserPrefix + userID))
	if err != nil {
		return nil, macaroonError(err)
	}

	return mac, nil
}

func macaroonError(err error) error {
	return fmt.Errorf("Macaroon creation failed: %s", err.Error())
}

// serializeMacaroon takes a macaroon to be serialized.
// returns a base64 encoded string, URL safe, can be sent via web, email, etc.
func serializeMacaroon(m macaroon.Macaroon) (string, error) {
	bin, err := m.MarshalBinary()

	if err != nil {
		return "", err
	}
	urlSafeEncode := base64.RawURLEncoding.EncodeToString(bin)

	return urlSafeEncode, nil
}

// deserializeMacaroon takes a base64 encoded string to deserialized.
// Returns a macaroon. On failure returns error with description.
func deserializeMacaroon(urlSafeEncode string) (*macaroon.Macaroon, error) {
	bin, err := base64.RawURLEncoding.DecodeString(urlSafeEncode)

	if err != nil {
		return nil, err
	}
	var mac *macaroon.Macaroon
	err = mac.UnmarshalBinary(bin)

	if err != nil {
		return nil, err
	}
	return mac, nil
}
