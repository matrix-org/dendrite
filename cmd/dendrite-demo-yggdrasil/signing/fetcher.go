// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package signing

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
)

const KeyID = "ed25519:dendrite-demo-yggdrasil"

type YggdrasilKeys struct {
}

func (f *YggdrasilKeys) KeyRing() *gomatrixserverlib.KeyRing {
	return &gomatrixserverlib.KeyRing{
		KeyDatabase: f,
	}
}

func (f *YggdrasilKeys) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	res := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult)
	for req := range requests {
		if req.KeyID != KeyID {
			return nil, fmt.Errorf("FetchKeys: cannot fetch key with ID %s, should be %s", req.KeyID, KeyID)
		}

		hexkey, err := hex.DecodeString(string(req.ServerName))
		if err != nil {
			return nil, fmt.Errorf("FetchKeys: can't decode server name %q: %w", req.ServerName, err)
		}

		res[req] = gomatrixserverlib.PublicKeyLookupResult{
			VerifyKey: gomatrixserverlib.VerifyKey{
				Key: hexkey,
			},
			ExpiredTS:    gomatrixserverlib.PublicKeyNotExpired,
			ValidUntilTS: gomatrixserverlib.AsTimestamp(time.Now().Add(24 * time.Hour * 365)),
		}
	}
	return res, nil
}

func (f *YggdrasilKeys) FetcherName() string {
	return "YggdrasilKeys"
}

func (f *YggdrasilKeys) StoreKeys(ctx context.Context, results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult) error {
	return nil
}
