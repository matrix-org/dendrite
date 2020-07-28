package config

type CurrentStateServer struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	// The CurrentState database stores the current state of all rooms.
	// It is accessed by the CurrentStateServer.
	Database        DataSource      `yaml:"database"`
	DatabaseOptions DatabaseOptions `yaml:"database_options"`
}

func (c *CurrentStateServer) Defaults() {
	c.Listen = "localhost:7782"
	c.Bind = "localhost:7782"
	c.Database = "file:currentstate.db"
	c.DatabaseOptions.Defaults()
}

func (c *CurrentStateServer) Verify(configErrs *configErrors) {
	checkNotEmpty(configErrs, "current_state_server.listen", string(c.Listen))
	checkNotEmpty(configErrs, "current_state_server.bind", string(c.Bind))
	checkNotEmpty(configErrs, "current_state_server.database", string(c.Database))
}
