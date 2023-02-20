package config

type KeyServer struct {
	Matrix *Global `yaml:"-"`

	Database DatabaseOptions `yaml:"database,omitempty"`
}

func (c *KeyServer) Defaults(opts DefaultOpts) {
	if opts.Generate {
		if !opts.SingleDatabase {
			c.Database.ConnectionString = "file:keyserver.db"
		}
	}
}

func (c *KeyServer) Verify(configErrs *ConfigErrors) {
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "key_server.database.connection_string", string(c.Database.ConnectionString))
	}
}
