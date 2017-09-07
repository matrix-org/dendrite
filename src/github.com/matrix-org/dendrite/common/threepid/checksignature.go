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

package threepid

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/matrix-org/gomatrixserverlib"
)

// CheckIDServerSignatures iterates over the signatures of a requests.
// If no signature can be found for the ID server's domain, returns an error, else
// iterates over the signature for the said domain, retrieves the matching public
// key, and verify it.
// Returns nil if all the verifications succeeded.
// Returns an error if something failed in the process.
func CheckIDServerSignatures(idServer string, signatures map[string]map[string]string, marshalledBody []byte) error {
	// TODO: Check if the domain is part of a list of trusted ID servers
	idServerSignatures, ok := signatures[idServer]
	if !ok {
		return errors.New("No signature for domain " + idServer)
	}

	for keyID := range idServerSignatures {
		pubKey, err := queryIDServerPubKey(idServer, keyID)
		if err != nil {
			return err
		}
		if err = gomatrixserverlib.VerifyJSON(idServer, gomatrixserverlib.KeyID(keyID), pubKey, marshalledBody); err != nil {
			return err
		}
	}

	return nil
}

// queryIDServerPubKey requests a public key identified with a given ID to the
// a given identity server and returns the matching base64-decoded public key.
// Returns an error if the request couldn't be sent, if its body couldn't be parsed
// or if the key couldn't be decoded from base64.
func queryIDServerPubKey(idServerName string, keyID string) ([]byte, error) {
	url := fmt.Sprintf("https://%s/_matrix/identity/api/v1/pubkey/%s", idServerName, keyID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	var pubKeyRes struct {
		PublicKey gomatrixserverlib.Base64String `json:"public_key"`
	}

	if resp.StatusCode != http.StatusOK {
		// TODO: Log the error supplied with the identity server?
		errMsg := fmt.Sprintf("Couldn't retrieve key %s from server %s", keyID, idServerName)
		return nil, errors.New(errMsg)
	}

	err = json.NewDecoder(resp.Body).Decode(&pubKeyRes)
	return pubKeyRes.PublicKey, err
}
