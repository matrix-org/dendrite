package storage

import (
	"net/url"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/matrix-org/dendrite/cmd/dendrite-p2p-demo/storage/postgreswithpubsub"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/publicroomsapi/storage/sqlite3"
)

const schemePostgres = "postgres"
const schemeFile = "file"

// NewPublicRoomsServerDatabase opens a database connection.
func NewPublicRoomsServerDatabaseWithPubSub(dataSourceName string, pubsub *pubsub.PubSub) (storage.Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgreswithpubsub.NewPublicRoomsServerDatabase(dataSourceName, pubsub)
	}
	switch uri.Scheme {
	case schemePostgres:
		return postgreswithpubsub.NewPublicRoomsServerDatabase(dataSourceName, pubsub)
	case schemeFile:
		return sqlite3.NewPublicRoomsServerDatabase(dataSourceName)
	default:
		return postgreswithpubsub.NewPublicRoomsServerDatabase(dataSourceName, pubsub)
	}
}
