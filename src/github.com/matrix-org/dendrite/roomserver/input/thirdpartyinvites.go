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

package input

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
)

// checkThirdPartyKeys checks the validity of all the public keys in a given
// m.room.third_party_invite event.
// Returns with an error if a key could not be verified.
func checkThirdPartyKeys(event gomatrixserverlib.Event) error {
	if event.Type() == "m.room.third_party_invite" {
		var content common.ThirdPartyInviteContent
		if err := json.Unmarshal(event.Content(), &content); err != nil {
			return err
		}

		for _, key := range content.PublicKeys {
			url := fmt.Sprintf("%s?public_key=%s", key.KeyValidityURL, key.PublicKey)
			resp, err := http.Get(url)
			if err != nil {
				return err
			}

			if resp.StatusCode != http.StatusOK {
				errMsg := fmt.Sprintf("Could not verify the validity of key %s at %s", key.PublicKey, key.KeyValidityURL)
				return errors.New(errMsg)
			}

			var validity struct {
				Valid bool `json:"valid"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&validity); err != nil {
				return err
			}

			if !validity.Valid {
				errMsg := fmt.Sprintf("Invalid key %s according to %s", key.PublicKey, key.KeyValidityURL)
				return errors.New(errMsg)
			}
		}
	}

	return nil
}
