package config

type RoomServer struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	Database DatabaseOptions `yaml:"database"`
}

func (c *RoomServer) Defaults(generate bool) {
	c.InternalAPI.Listen = "http://localhost:7770"
	c.InternalAPI.Connect = "http://localhost:7770"
	c.Database.Defaults(10)
	if generate {
		c.Database.ConnectionString = "file:roomserver.db"
	}
}

func (c *RoomServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "room_server.database.connection_string", string(c.Database.ConnectionString))
	}
	if isMonolith { // polylith required configs below
		return
	}
	checkURL(configErrs, "room_server.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "room_server.internal_ap.connect", string(c.InternalAPI.Connect))
}
