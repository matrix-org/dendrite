// Copyright 2017 Vector Creations Ltd
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
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/ed25519"
	"io"
	"strings"
)

// ReadKey reads a server's private ed25519 key.
// If the key is the empty string then a random key is generated.
// Otherwise the key is the key ID and the base64 encoded private key
// separated by a single space character.
// E.g "ed25519:abcd ABCDEFGHIJKLMNOPabcdefghijklmnop01234567890"
func ReadKey(key string) (gomatrixserverlib.KeyID, ed25519.PrivateKey, error) {
	var keyID gomatrixserverlib.KeyID
	var seed io.Reader
	if key == "" {
		// TODO: We should fail if we don't have a private key rather than
		// generating a throw away key.
		keyID = gomatrixserverlib.KeyID("ed25519:something")
	} else {
		// TODO: We should be reading this from a PEM formatted file instead of
		// reading from the environment directly.
		parts := strings.SplitN(key, " ", 2)
		keyID = gomatrixserverlib.KeyID(parts[0])
		if len(parts) != 2 {
			return "", nil, fmt.Errorf("Invalid server key: %q", key)
		}
		seedBytes, err := base64.RawStdEncoding.DecodeString(parts[1])
		if err != nil {
			return "", nil, err
		}
		seed = bytes.NewReader(seedBytes)
	}
	_, privKey, err := ed25519.GenerateKey(seed)
	if err != nil {
		return "", nil, err
	}
	return keyID, privKey, nil
}
