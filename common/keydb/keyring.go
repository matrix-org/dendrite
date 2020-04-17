// Copyright 2017 New Vector Ltd
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

package keydb

import (
	"encoding/base64"

	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/ed25519"
)

// CreateKeyRing creates and configures a KeyRing object.
//
// It creates the necessary key fetchers and collects them into a KeyRing
// backed by the given KeyDatabase.
func CreateKeyRing(client gomatrixserverlib.Client,
	keyDB gomatrixserverlib.KeyDatabase) gomatrixserverlib.KeyRing {

	var b64e = base64.StdEncoding.WithPadding(base64.NoPadding)
	matrixOrgKey1, _ := b64e.DecodeString("Noi6WqcDj0QmPxCNQqgezwTlBKrfqehY1u2FyWP9uYw")
	matrixOrgKey2, _ := b64e.DecodeString("l8Hft5qXKn1vfHrg3p4+W8gELQVo8N13JkluMfmn2sQ")

	return gomatrixserverlib.KeyRing{
		KeyFetchers: []gomatrixserverlib.KeyFetcher{
			// TODO: Use perspective key fetchers for production.
			//&gomatrixserverlib.DirectKeyFetcher{
			//	Client: client,
			//},
			&gomatrixserverlib.PerspectiveKeyFetcher{
				PerspectiveServerName: "matrix.org",
				PerspectiveServerKeys: map[gomatrixserverlib.KeyID]ed25519.PublicKey{
					"ed25519:auto":   matrixOrgKey1,
					"ed25519:a_RXGa": matrixOrgKey2,
				},
				Client: client,
			},
		},
		KeyDatabase: keyDB,
	}
}
