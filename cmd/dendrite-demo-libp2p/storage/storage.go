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

package storage

import (
	"net/url"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-libp2p/storage/postgreswithdht"
	"github.com/matrix-org/dendrite/cmd/dendrite-demo-libp2p/storage/postgreswithpubsub"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/publicroomsapi/storage/sqlite3"
	"github.com/matrix-org/gomatrixserverlib"
)

const schemePostgres = "postgres"
const schemeFile = "file"

// NewPublicRoomsServerDatabase opens a database connection.
func NewPublicRoomsServerDatabaseWithDHT(dataSourceName string, dht *dht.IpfsDHT, localServerName gomatrixserverlib.ServerName) (storage.Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgreswithdht.NewPublicRoomsServerDatabase(dataSourceName, dht, localServerName)
	}
	switch uri.Scheme {
	case schemePostgres:
		return postgreswithdht.NewPublicRoomsServerDatabase(dataSourceName, dht, localServerName)
	case schemeFile:
		return sqlite3.NewPublicRoomsServerDatabase(dataSourceName, localServerName)
	default:
		return postgreswithdht.NewPublicRoomsServerDatabase(dataSourceName, dht, localServerName)
	}
}

// NewPublicRoomsServerDatabase opens a database connection.
func NewPublicRoomsServerDatabaseWithPubSub(dataSourceName string, pubsub *pubsub.PubSub, localServerName gomatrixserverlib.ServerName) (storage.Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgreswithpubsub.NewPublicRoomsServerDatabase(dataSourceName, pubsub, localServerName)
	}
	switch uri.Scheme {
	case schemePostgres:
		return postgreswithpubsub.NewPublicRoomsServerDatabase(dataSourceName, pubsub, localServerName)
	case schemeFile:
		return sqlite3.NewPublicRoomsServerDatabase(dataSourceName, localServerName)
	default:
		return postgreswithpubsub.NewPublicRoomsServerDatabase(dataSourceName, pubsub, localServerName)
	}
}
