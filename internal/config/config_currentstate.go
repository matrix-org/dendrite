package config

type CurrentStateServer struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	// The CurrentState database stores the current state of all rooms.
	// It is accessed by the CurrentStateServer.
	Database DatabaseOptions `yaml:"database"`
}

func (c *CurrentStateServer) Defaults() {
	c.Listen = "localhost:7782"
	c.Bind = "localhost:7782"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:currentstate.db"
}

func (c *CurrentStateServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "current_state_server.listen", string(c.Listen))
	checkNotEmpty(configErrs, "current_state_server.bind", string(c.Bind))
	checkNotEmpty(configErrs, "current_state_server.database.connection_string", string(c.Database.ConnectionString))
}
