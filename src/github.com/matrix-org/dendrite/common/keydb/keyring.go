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

import "github.com/matrix-org/gomatrixserverlib"

// CreateKeyRing creates and configures a KeyRing object.
//
// It creates the necessary key fetchers and collects them into a KeyRing
// backed by the given KeyDatabase.
func CreateKeyRing(client gomatrixserverlib.Client,
	keyDB gomatrixserverlib.KeyDatabase) gomatrixserverlib.KeyRing {
	return gomatrixserverlib.KeyRing{
		KeyFetchers: []gomatrixserverlib.KeyFetcher{
			// TODO: Use perspective key fetchers for production.
			&gomatrixserverlib.DirectKeyFetcher{Client: client},
		},
		KeyDatabase: keyDB,
	}
}
