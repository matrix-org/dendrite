package config

import "github.com/matrix-org/gomatrixserverlib"

type SigningKeyServer struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	// The SigningKeyServer database caches the public keys of remote servers.
	// It may be accessed by the FederationAPI, the ClientAPI, and the MediaAPI.
	Database DatabaseOptions `yaml:"database"`

	// Perspective keyservers, to use as a backup when direct key fetch
	// requests don't succeed
	KeyPerspectives KeyPerspectives `yaml:"key_perspectives"`

	// Should we prefer direct key fetches over perspective ones?
	PreferDirectFetch bool `yaml:"prefer_direct_fetch"`
}

func (c *SigningKeyServer) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7780"
	c.InternalAPI.Connect = "http://localhost:7780"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:signingkeyserver.db"
}

func (c *SigningKeyServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "signing_key_server.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "signing_key_server.internal_api.bind", string(c.InternalAPI.Connect))
	checkNotEmpty(configErrs, "signing_key_server.database.connection_string", string(c.Database.ConnectionString))
}

// KeyPerspectives are used to configure perspective key servers for
// retrieving server keys.
type KeyPerspectives []KeyPerspective

type KeyPerspective struct {
	// The server name of the perspective key server
	ServerName gomatrixserverlib.ServerName `yaml:"server_name"`
	// Server keys for the perspective user, used to verify the
	// keys have been signed by the perspective server
	Keys []KeyPerspectiveTrustKey `yaml:"keys"`
}

type KeyPerspectiveTrustKey struct {
	// The key ID, e.g. ed25519:auto
	KeyID gomatrixserverlib.KeyID `yaml:"key_id"`
	// The public key in base64 unpadded format
	PublicKey string `yaml:"public_key"`
}
