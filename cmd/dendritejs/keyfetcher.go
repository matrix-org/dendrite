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

// +build wasm

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const libp2pMatrixKeyID = "ed25519:libp2p-dendrite"

type libp2pKeyFetcher struct {
}

// FetchKeys looks up a batch of public keys.
// Takes a map from (server name, key ID) pairs to timestamp.
// The timestamp is when the keys need to be vaild up to.
// Returns a map from (server name, key ID) pairs to server key objects for
// that server name containing that key ID
// The result may have fewer (server name, key ID) pairs than were in the request.
// The result may have more (server name, key ID) pairs than were in the request.
// Returns an error if there was a problem fetching the keys.
func (f *libp2pKeyFetcher) FetchKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	res := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult)
	for req := range requests {
		if req.KeyID != libp2pMatrixKeyID {
			return nil, fmt.Errorf("FetchKeys: cannot fetch key with ID %s, should be %s", req.KeyID, libp2pMatrixKeyID)
		}

		// The server name is a libp2p peer ID
		peerIDStr := string(req.ServerName)
		peerID, err := peer.Decode(peerIDStr)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode peer ID from server name '%s': %w", peerIDStr, err)
		}
		pubKey, err := peerID.ExtractPublicKey()
		if err != nil {
			return nil, fmt.Errorf("Failed to extract public key from peer ID: %w", err)
		}
		pubKeyBytes, err := pubKey.Raw()
		if err != nil {
			return nil, fmt.Errorf("Failed to extract raw bytes from public key: %w", err)
		}
		util.GetLogger(ctx).Info("libp2pKeyFetcher.FetchKeys: Using public key %v for server name %s", pubKeyBytes, req.ServerName)

		b64Key := gomatrixserverlib.Base64String(pubKeyBytes)
		res[req] = gomatrixserverlib.PublicKeyLookupResult{
			VerifyKey: gomatrixserverlib.VerifyKey{
				Key: b64Key,
			},
			ExpiredTS:    gomatrixserverlib.PublicKeyNotExpired,
			ValidUntilTS: gomatrixserverlib.AsTimestamp(time.Now().Add(24 * time.Hour * 365)),
		}
	}
	return res, nil
}

// FetcherName returns the name of this fetcher, which can then be used for
// logging errors etc.
func (f *libp2pKeyFetcher) FetcherName() string {
	return "libp2pKeyFetcher"
}

// no-op function for storing keys - we don't do any work to fetch them so don't bother storing.
func (f *libp2pKeyFetcher) StoreKeys(ctx context.Context, results map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult) error {
	return nil
}
