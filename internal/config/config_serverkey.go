package config

type ServerKeyAPI struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	// The ServerKey database caches the public keys of remote servers.
	// It may be accessed by the FederationAPI, the ClientAPI, and the MediaAPI.
	Database DatabaseOptions `yaml:"database"`

	// Perspective keyservers, to use as a backup when direct key fetch
	// requests don't succeed
	KeyPerspectives KeyPerspectives `yaml:"key_perspectives"`
}

func (c *ServerKeyAPI) Defaults() {
	c.Listen = "localhost:7780"
	c.Bind = "localhost:7780"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:serverkeyapi.db"
}

func (c *ServerKeyAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "server_key_api.listen", string(c.Listen))
	checkNotEmpty(configErrs, "server_key_api.bind", string(c.Bind))
	checkNotEmpty(configErrs, "server_key_api.database.connection_string", string(c.Database.ConnectionString))
}
