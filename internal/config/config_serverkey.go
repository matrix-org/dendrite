package config

import "github.com/matrix-org/gomatrixserverlib"

type ServerKeyAPI struct {
	Matrix *Global `json:"-"`

	Listen          Address         `json:"Listen" comment:"Listen address for this component."`
	Bind            Address         `json:"Bind" comment:"Bind address for this component."`
	Database        DatabaseOptions `json:"Database" comment:"Database configuration for this component."`
	KeyPerspectives KeyPerspectives `json:"PerspectiveServers" comment:"Perspective keyservers to use as a backup when direct key fetches fail."`
}

func (c *ServerKeyAPI) Defaults() {
	c.Listen = "localhost:7780"
	c.Bind = "localhost:7780"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:serverkeyapi.db"
}

func (c *ServerKeyAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "server_key_api.listen", string(c.Listen))
	checkNotEmpty(configErrs, "server_key_api.bind", string(c.Bind))
	checkNotEmpty(configErrs, "server_key_api.database.connection_string", string(c.Database.ConnectionString))
}

// KeyPerspectives are used to configure perspective key servers for
// retrieving server keys.
type KeyPerspectives []KeyPerspective

// KeyPerspectiveTrustKeys denote the signature keys used to trust results.
type KeyPerspectiveTrustKeys struct {
	// The key ID, e.g. ed25519:auto
	KeyID gomatrixserverlib.KeyID `json:"KeyID"`
	// The public key in base64 unpadded format
	PublicKey string `json:"PublicKey"`
}

// KeyPerspective is used to configure a key perspective server.
type KeyPerspective struct {
	// The server name of the perspective key server
	ServerName gomatrixserverlib.ServerName `json:"ServerName"`
	// Server keys for the perspective user, used to verify the
	// keys have been signed by the perspective server
	Keys []KeyPerspectiveTrustKeys `json:"TrustKeys"`
}
