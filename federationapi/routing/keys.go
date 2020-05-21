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

package routing

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"golang.org/x/crypto/ed25519"
)

// LocalKeys returns the local keys for the server.
// See https://matrix.org/docs/spec/server_server/unstable.html#publishing-keys
func LocalKeys(cfg *config.Dendrite) util.JSONResponse {
	keys, err := localKeys(cfg, time.Now().Add(cfg.Matrix.KeyValidityPeriod))
	if err != nil {
		return util.ErrorResponse(err)
	}
	return util.JSONResponse{Code: http.StatusOK, JSON: keys}
}

func localKeys(cfg *config.Dendrite, validUntil time.Time) (*gomatrixserverlib.ServerKeys, error) {
	var keys gomatrixserverlib.ServerKeys

	keys.ServerName = cfg.Matrix.ServerName

	publicKey := cfg.Matrix.PrivateKey.Public().(ed25519.PublicKey)

	keys.VerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.VerifyKey{
		cfg.Matrix.KeyID: {
			Key: gomatrixserverlib.Base64String(publicKey),
		},
	}

	keys.TLSFingerprints = cfg.Matrix.TLSFingerPrints
	keys.OldVerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.OldVerifyKey{}
	keys.ValidUntilTS = gomatrixserverlib.AsTimestamp(validUntil)

	toSign, err := json.Marshal(keys.ServerKeyFields)
	if err != nil {
		return nil, err
	}

	keys.Raw, err = gomatrixserverlib.SignJSON(
		string(cfg.Matrix.ServerName), cfg.Matrix.KeyID, cfg.Matrix.PrivateKey, toSign,
	)
	if err != nil {
		return nil, err
	}

	return &keys, nil
}
