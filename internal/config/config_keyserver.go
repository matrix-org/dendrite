package config

type KeyServer struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	Database DatabaseOptions `yaml:"database"`
}

func (c *KeyServer) Defaults() {
	c.Listen = "localhost:7779"
	c.Bind = "localhost:7779"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:keyserver.db"
}

func (c *KeyServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "key_server.listen", string(c.Listen))
	checkNotEmpty(configErrs, "key_server.bind", string(c.Bind))
	checkNotEmpty(configErrs, "key_server.database.connection_string", string(c.Database.ConnectionString))
}
