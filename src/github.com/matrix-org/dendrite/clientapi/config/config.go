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
