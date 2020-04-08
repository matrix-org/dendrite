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

// +build !wasm

package storage

import (
	"net/url"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/matrix-org/dendrite/publicroomsapi/storage/postgres"
	"github.com/matrix-org/dendrite/publicroomsapi/storage/postgreswithdht"
	"github.com/matrix-org/dendrite/publicroomsapi/storage/postgreswithpubsub"
	"github.com/matrix-org/dendrite/publicroomsapi/storage/sqlite3"
)

// NewPublicRoomsServerDatabase opens a database connection.
func NewPublicRoomsServerDatabase(dataSourceName string) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.NewPublicRoomsServerDatabase(dataSourceName)
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.NewPublicRoomsServerDatabase(dataSourceName)
	case "file":
		return sqlite3.NewPublicRoomsServerDatabase(dataSourceName)
	default:
		return postgres.NewPublicRoomsServerDatabase(dataSourceName)
	}
}

// NewPublicRoomsServerDatabase opens a database connection.
func NewPublicRoomsServerDatabaseWithDHT(dataSourceName string, dht *dht.IpfsDHT) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgreswithdht.NewPublicRoomsServerDatabase(dataSourceName, dht)
	}
	switch uri.Scheme {
	case "postgres":
		return postgreswithdht.NewPublicRoomsServerDatabase(dataSourceName, dht)
	case "file":
		return sqlite3.NewPublicRoomsServerDatabase(dataSourceName)
	default:
		return postgreswithdht.NewPublicRoomsServerDatabase(dataSourceName, dht)
	}
}

// NewPublicRoomsServerDatabase opens a database connection.
func NewPublicRoomsServerDatabaseWithPubSub(dataSourceName string, pubsub *pubsub.PubSub) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgreswithpubsub.NewPublicRoomsServerDatabase(dataSourceName, pubsub)
	}
	switch uri.Scheme {
	case "postgres":
		return postgreswithpubsub.NewPublicRoomsServerDatabase(dataSourceName, pubsub)
	case "file":
		return sqlite3.NewPublicRoomsServerDatabase(dataSourceName)
	default:
		return postgreswithpubsub.NewPublicRoomsServerDatabase(dataSourceName, pubsub)
	}
}
