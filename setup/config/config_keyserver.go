package config

type KeyServer struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	Database DatabaseOptions `yaml:"database"`
}

func (c *KeyServer) Defaults(generate bool) {
	c.InternalAPI.Listen = "http://localhost:7779"
	c.InternalAPI.Connect = "http://localhost:7779"
	c.Database.Defaults(10)
	if generate {
		c.Database.ConnectionString = "file:keyserver.db"
	}
}

func (c *KeyServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "key_server.database.connection_string", string(c.Database.ConnectionString))
	}
	if isMonolith { // polylith required configs below
		return
	}
	checkURL(configErrs, "key_server.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "key_server.internal_api.connect", string(c.InternalAPI.Connect))
}
