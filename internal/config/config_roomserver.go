package config

type RoomServer struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	Database        DataSource      `yaml:"database"`
	DatabaseOptions DatabaseOptions `yaml:"database_options"`
}

func (c *RoomServer) Defaults() {
	c.Listen = "localhost:7770"
	c.Bind = "localhost:7770"
	c.Database = "file:roomserver.db"
	c.DatabaseOptions.Defaults()
}

func (c *RoomServer) Verify(configErrs *configErrors) {
	checkNotEmpty(configErrs, "room_server.listen", string(c.Listen))
	checkNotEmpty(configErrs, "room_server.bind", string(c.Bind))
	checkNotEmpty(configErrs, "room_server.database", string(c.Database))
}
