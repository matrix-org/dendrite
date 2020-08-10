package config

type FederationSender struct {
	Matrix *Global `json:"-"`

	Listen               Address         `json:"listen"`
	Bind                 Address         `json:"bind"`
	Database             DatabaseOptions `json:"Database" comment:"Database configuration for this component."`
	FederationMaxRetries uint32          `json:"SendMaxRetries" comment:"How many times we will try to resend a failed transaction to a specific server. The\nbackoff is 2**x seconds, so 1 = 2 seconds, 2 = 4 seconds, 3 = 8 seconds etc."`
	DisableTLSValidation bool            `json:"DisableTLSValidation" comment:"Disable the validation of TLS certificates of remote federated homeservers. Do not\nenable this option in production as it presents a security risk!"`
	Proxy                Proxy           `json:"ProxyOutbound" comment:"Use the following proxy server for outbound federation traffic."`
}

func (c *FederationSender) Defaults() {
	c.Listen = "localhost:7775"
	c.Bind = "localhost:7775"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:federationsender.db"

	c.FederationMaxRetries = 16
	c.DisableTLSValidation = false

	c.Proxy.Defaults()
}

func (c *FederationSender) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "federation_sender.listen", string(c.Listen))
	checkNotEmpty(configErrs, "federation_sender.bind", string(c.Bind))
	checkNotEmpty(configErrs, "federation_sender.database.connection_string", string(c.Database.ConnectionString))
}

// The config for setting a proxy to use for server->server requests
type Proxy struct {
	// Is the proxy enabled?
	Enabled bool `json:"Enabled"`
	// The protocol for the proxy (http / https / socks5)
	Protocol string `json:"Protocol"`
	// The host where the proxy is listening
	Host string `json:"Host"`
	// The port on which the proxy is listening
	Port uint16 `json:"Port"`
}

func (c *Proxy) Defaults() {
	c.Enabled = false
	c.Protocol = "http"
	c.Host = "localhost"
	c.Port = 8080
}

func (c *Proxy) Verify(configErrs *ConfigErrors) {
}
