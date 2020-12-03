package config

type KeyServer struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	Database DatabaseOptions `yaml:"database"`
}

func (c *KeyServer) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7779"
	c.InternalAPI.Connect = "http://localhost:7779"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:keyserver.db"
}

func (c *KeyServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "key_server.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "key_server.internal_api.bind", string(c.InternalAPI.Connect))
	checkNotEmpty(configErrs, "key_server.database.connection_string", string(c.Database.ConnectionString))
}
