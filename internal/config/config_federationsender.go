package config

type FederationSender struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	// The FederationSender database stores information used by the FederationSender
	// It is only accessed by the FederationSender.
	Database        DataSource      `yaml:"database"`
	DatabaseOptions DatabaseOptions `yaml:"database_options"`

	// Federation failure threshold. How many consecutive failures that we should
	// tolerate when sending federation requests to a specific server. The backoff
	// is 2**x seconds, so 1 = 2 seconds, 2 = 4 seconds, 3 = 8 seconds, etc.
	// The default value is 16 if not specified, which is circa 18 hours.
	FederationMaxRetries uint32 `yaml:"federation_max_retries"`

	Proxy Proxy `yaml:"proxy"`
}

func (c *FederationSender) Defaults() {
	c.Listen = "localhost:7775"
	c.Bind = "localhost:7775"
	c.Database = "file:federationsender.db"
	c.DatabaseOptions.Defaults()

	c.FederationMaxRetries = 16
}

func (c *FederationSender) Verify(configErrs *configErrors) {
	checkNotEmpty(configErrs, "federation_sender.listen", string(c.Listen))
	checkNotEmpty(configErrs, "federation_sender.bind", string(c.Bind))
	checkNotEmpty(configErrs, "federation_sender.database", string(c.Database))
}

// The config for setting a proxy to use for server->server requests
type Proxy struct {
	// The protocol for the proxy (http / https / socks5)
	Protocol string `yaml:"protocol"`
	// The host where the proxy is listening
	Host string `yaml:"host"`
	// The port on which the proxy is listening
	Port uint16 `yaml:"port"`
}

func (c *Proxy) Defaults() {
	c.Protocol = ""
	c.Host = ""
	c.Port = 0
}

func (c *Proxy) Verify(configErrs *configErrors) {
}
