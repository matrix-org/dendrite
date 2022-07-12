package config

type KeyServer struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api,omitempty"`

	Database DatabaseOptions `yaml:"database,omitempty"`
}

func (c *KeyServer) Defaults(opts DefaultOpts) {
	if !opts.Monolithic {
		c.InternalAPI.Listen = "http://localhost:7779"
		c.InternalAPI.Connect = "http://localhost:7779"
		c.Database.Defaults(10)
	}
	if opts.Generate {
		if !opts.Monolithic {
			c.Database.ConnectionString = "file:keyserver.db"
		}
	}
}

func (c *KeyServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if isMonolith { // polylith required configs below
		return
	}
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "key_server.database.connection_string", string(c.Database.ConnectionString))
	}
	checkURL(configErrs, "key_server.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "key_server.internal_api.connect", string(c.InternalAPI.Connect))
}
