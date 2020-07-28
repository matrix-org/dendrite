package config

type ServerKeyAPI struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	// The ServerKey database caches the public keys of remote servers.
	// It may be accessed by the FederationAPI, the ClientAPI, and the MediaAPI.
	Database        DataSource      `yaml:"database"`
	DatabaseOptions DatabaseOptions `yaml:"database_options"`

	// Perspective keyservers, to use as a backup when direct key fetch
	// requests don't succeed
	KeyPerspectives KeyPerspectives `yaml:"key_perspectives"`
}

func (c *ServerKeyAPI) Defaults() {
	c.Listen = "localhost:7780"
	c.Bind = "localhost:7780"
	c.Database = "file:serverkeyapi.db"
	c.DatabaseOptions.Defaults()
}

func (c *ServerKeyAPI) Verify(configErrs *configErrors) {
	checkNotEmpty(configErrs, "server_key_api.listen", string(c.Listen))
	checkNotEmpty(configErrs, "server_key_api.bind", string(c.Bind))
	checkNotEmpty(configErrs, "server_key_api.database", string(c.Database))
}
