package config

type CurrentStateServer struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	// The CurrentState database stores the current state of all rooms.
	// It is accessed by the CurrentStateServer.
	Database DatabaseOptions `yaml:"database"`
}

func (c *CurrentStateServer) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7782"
	c.InternalAPI.Connect = "http://localhost:7782"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:currentstate.db"
}

func (c *CurrentStateServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "current_state_server.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "current_state_server.internal_api.connect", string(c.InternalAPI.Connect))
	checkNotEmpty(configErrs, "current_state_server.database.connection_string", string(c.Database.ConnectionString))
}
