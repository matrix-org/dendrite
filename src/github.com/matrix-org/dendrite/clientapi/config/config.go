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

// Sync contains the config information necessary to spin up a sync-server process.
type Sync struct {
	// The topic for events which are written by the room server output log.
	RoomserverOutputTopic string `yaml:"roomserver_topic"`
	// A list of URIs to consume events from. These kafka logs should be produced by a Room Server.
	KafkaConsumerURIs []string `yaml:"consumer_uris"`
	// The postgres connection config for connecting to the database e.g a postgres:// URI
	DataSource string `yaml:"database"`
}
