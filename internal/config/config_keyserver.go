package config

type KeyServer struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	Database        DataSource      `yaml:"database"`
	DatabaseOptions DatabaseOptions `yaml:"database_options"`
}

func (c *KeyServer) Defaults() {
	c.Listen = "localhost:7779"
	c.Bind = "localhost:7779"
	c.Database = "file:keyserver.db"
	c.DatabaseOptions.Defaults()
}

func (c *KeyServer) Verify(configErrs *configErrors) {
	checkNotEmpty(configErrs, "key_server.listen", string(c.Listen))
	checkNotEmpty(configErrs, "key_server.bind", string(c.Bind))
	checkNotEmpty(configErrs, "key_server.database", string(c.Database))
}
