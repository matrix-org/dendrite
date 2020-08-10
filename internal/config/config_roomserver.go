package config

type RoomServer struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	Database DatabaseOptions `yaml:"database"`
}

func (c *RoomServer) Defaults() {
	c.Listen = "localhost:7770"
	c.Bind = "localhost:7770"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:roomserver.db"
}

func (c *RoomServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "room_server.listen", string(c.Listen))
	checkNotEmpty(configErrs, "room_server.bind", string(c.Bind))
	checkNotEmpty(configErrs, "room_server.database.connection_string", string(c.Database.ConnectionString))
}
