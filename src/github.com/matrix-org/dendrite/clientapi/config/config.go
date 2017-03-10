package config

import "golang.org/x/crypto/ed25519"

// ClientAPI contains the config information necessary to spin up a clientapi process.
type ClientAPI struct {
	ServerName string
	PrivateKey ed25519.PrivateKey
	KeyID      string
}
