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
	"context"
	"encoding/base64"

	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
)

// CreateKeyRing creates and configures a KeyRing object.
//
// It creates the necessary key fetchers and collects them into a KeyRing
// backed by the given KeyDatabase.
func CreateKeyRing(client gomatrixserverlib.Client,
	keyDB gomatrixserverlib.KeyDatabase,
	cfg *config.Dendrite) gomatrixserverlib.KeyRing {

	fetchers := gomatrixserverlib.KeyRing{
		KeyFetchers: []gomatrixserverlib.KeyFetcher{
			&gomatrixserverlib.DirectKeyFetcher{
				Client: client,
			},
		},
		KeyDatabase: keyDB,
	}

	util.GetLogger(context.TODO()).Info("Enabled direct key fetcher")

	var b64e = base64.StdEncoding.WithPadding(base64.NoPadding)
	for _, ps := range cfg.Matrix.KeyPerspectives {
		perspective := &gomatrixserverlib.PerspectiveKeyFetcher{
			PerspectiveServerName: ps.ServerName,
			PerspectiveServerKeys: map[gomatrixserverlib.KeyID]ed25519.PublicKey{},
			Client:                client,
		}

		for _, key := range ps.Keys {
			rawkey, err := b64e.DecodeString(key.PublicKey)
			if err != nil {
				util.GetLogger(context.TODO()).WithError(err).WithFields(logrus.Fields{
					"server_name": ps.ServerName,
					"public_key":  key.PublicKey,
				}).Warn("Couldn't parse perspective key")
				continue
			}
			perspective.PerspectiveServerKeys[key.KeyID] = rawkey
		}

		util.GetLogger(context.TODO()).WithField("server_name", ps.ServerName).Info("Enabled perspective key fetcher")
	}

	return fetchers
}
