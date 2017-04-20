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

import "golang.org/x/crypto/ed25519"

// ClientAPI contains the config information necessary to spin up a clientapi process.
type ClientAPI struct {
	// The name of the server. This is usually the domain name, e.g 'matrix.org', 'localhost'.
	ServerName string
	// The private key which will be used to sign events.
	PrivateKey ed25519.PrivateKey
	// An arbitrary string used to uniquely identify the PrivateKey. Must start with the
	// prefix "ed25519:".
	KeyID string
	// A list of URIs to send events to. These kafka logs should be consumed by a Room Server.
	KafkaProducerURIs []string
	// The topic for events which are written to the logs.
	ClientAPIOutputTopic string
	// The URL of the roomserver which can service Query API requests
	RoomserverURL string
}
