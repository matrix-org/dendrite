package config

import "github.com/matrix-org/gomatrixserverlib"

type FederationAPI struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api,omitempty"`
	ExternalAPI ExternalAPIOptions `yaml:"external_api,omitempty"`

	// The database stores information used by the federation destination queues to
	// send transactions to remote servers.
	Database DatabaseOptions `yaml:"database,omitempty"`

	// Federation failure threshold. How many consecutive failures that we should
	// tolerate when sending federation requests to a specific server. The backoff
	// is 2**x seconds, so 1 = 2 seconds, 2 = 4 seconds, 3 = 8 seconds, etc.
	// The default value is 16 if not specified, which is circa 18 hours.
	FederationMaxRetries uint32 `yaml:"send_max_retries"`

	// FederationDisableTLSValidation disables the validation of X.509 TLS certs
	// on remote federation endpoints. This is not recommended in production!
	DisableTLSValidation bool `yaml:"disable_tls_validation"`

	// DisableHTTPKeepalives prevents Dendrite from keeping HTTP connections
	// open for reuse for future requests. Connections will be closed quicker
	// but we may spend more time on TLS handshakes instead.
	DisableHTTPKeepalives bool `yaml:"disable_http_keepalives"`

	// Perspective keyservers, to use as a backup when direct key fetch
	// requests don't succeed
	KeyPerspectives KeyPerspectives `yaml:"key_perspectives"`

	// Should we prefer direct key fetches over perspective ones?
	PreferDirectFetch bool `yaml:"prefer_direct_fetch"`
}

func (c *FederationAPI) Defaults(opts DefaultOpts) {
	if !opts.Monolithic {
		c.InternalAPI.Listen = "http://localhost:7772"
		c.InternalAPI.Connect = "http://localhost:7772"
		c.ExternalAPI.Listen = "http://[::]:8072"
		c.Database.Defaults(10)
	}
	c.FederationMaxRetries = 16
	c.DisableTLSValidation = false
	c.DisableHTTPKeepalives = false
	if opts.Generate {
		c.KeyPerspectives = KeyPerspectives{
			{
				ServerName: "matrix.org",
				Keys: []KeyPerspectiveTrustKey{
					{
						KeyID:     "ed25519:auto",
						PublicKey: "Noi6WqcDj0QmPxCNQqgezwTlBKrfqehY1u2FyWP9uYw",
					},
					{
						KeyID:     "ed25519:a_RXGa",
						PublicKey: "l8Hft5qXKn1vfHrg3p4+W8gELQVo8N13JkluMfmn2sQ",
					},
				},
			},
		}
		if !opts.Monolithic {
			c.Database.ConnectionString = "file:federationapi.db"
		}
	}
}

func (c *FederationAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if isMonolith { // polylith required configs below
		return
	}
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "federation_api.database.connection_string", string(c.Database.ConnectionString))
	}
	checkURL(configErrs, "federation_api.external_api.listen", string(c.ExternalAPI.Listen))
	checkURL(configErrs, "federation_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "federation_api.internal_api.connect", string(c.InternalAPI.Connect))
}

// The config for setting a proxy to use for server->server requests
type Proxy struct {
	// Is the proxy enabled?
	Enabled bool `yaml:"enabled"`
	// The protocol for the proxy (http / https / socks5)
	Protocol string `yaml:"protocol"`
	// The host where the proxy is listening
	Host string `yaml:"host"`
	// The port on which the proxy is listening
	Port uint16 `yaml:"port"`
}

func (c *Proxy) Defaults() {
	c.Enabled = false
	c.Protocol = "http"
	c.Host = "localhost"
	c.Port = 8080
}

func (c *Proxy) Verify(configErrs *ConfigErrors) {
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
