package config

type RelayAPI struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api,omitempty"`
	ExternalAPI ExternalAPIOptions `yaml:"external_api,omitempty"`

	// The database stores information used by the relay queue to
	// forward transactions to remote servers.
	Database DatabaseOptions `yaml:"database,omitempty"`
}

func (c *RelayAPI) Defaults(opts DefaultOpts) {
	if !opts.Monolithic {
		c.InternalAPI.Listen = "http://localhost:7775"
		c.InternalAPI.Connect = "http://localhost:7775"
		c.ExternalAPI.Listen = "http://[::]:8075"
		c.Database.Defaults(10)
	}
	if opts.Generate {
		if !opts.Monolithic {
			c.Database.ConnectionString = "file:relayapi.db"
		}
	}
}

func (c *RelayAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if isMonolith { // polylith required configs below
		return
	}
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "relay_api.database.connection_string", string(c.Database.ConnectionString))
	}
	checkURL(configErrs, "relay_api.external_api.listen", string(c.ExternalAPI.Listen))
	checkURL(configErrs, "relay_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "relay_api.internal_api.connect", string(c.InternalAPI.Connect))
}
