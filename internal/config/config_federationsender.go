package config

type FederationSender struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	// The FederationSender database stores information used by the FederationSender
	// It is only accessed by the FederationSender.
	Database DatabaseOptions `yaml:"database"`

	// Federation failure threshold. How many consecutive failures that we should
	// tolerate when sending federation requests to a specific server. The backoff
	// is 2**x seconds, so 1 = 2 seconds, 2 = 4 seconds, 3 = 8 seconds, etc.
	// The default value is 16 if not specified, which is circa 18 hours.
	FederationMaxRetries uint32 `yaml:"send_max_retries"`

	// FederationDisableTLSValidation disables the validation of X.509 TLS certs
	// on remote federation endpoints. This is not recommended in production!
	DisableTLSValidation bool `yaml:"disable_tls_validation"`

	Proxy Proxy `yaml:"proxy_outbound"`
}

func (c *FederationSender) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7775"
	c.InternalAPI.Connect = "http://localhost:7775"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:federationsender.db"

	c.FederationMaxRetries = 16
	c.DisableTLSValidation = false

	c.Proxy.Defaults()
}

func (c *FederationSender) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "federation_sender.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "federation_sender.internal_api.connect", string(c.InternalAPI.Connect))
	checkNotEmpty(configErrs, "federation_sender.database.connection_string", string(c.Database.ConnectionString))
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
