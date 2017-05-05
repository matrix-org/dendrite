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

package config

import (
	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/ed25519"
	"time"
)

// FederationAPI contains the config information necessary to spin up a federationapi process.
type FederationAPI struct {
	// The name of the server. This is usually the domain name, e.g 'matrix.org', 'localhost'.
	ServerName gomatrixserverlib.ServerName
	// The private key which will be used to sign requests.
	PrivateKey ed25519.PrivateKey
	// An arbitrary string used to uniquely identify the PrivateKey. Must start with the
	// prefix "ed25519:".
	KeyID gomatrixserverlib.KeyID
	// A list of SHA256 TLS fingerprints for this server.
	TLSFingerPrints []gomatrixserverlib.TLSFingerprint
	// How long the keys are valid for.
	ValidityPeriod time.Duration
}
