package config

type RoomServer struct {
	Matrix *Global `json:"-"`

	Listen   Address         `json:"Listen" comment:"Listen address for this component."`
	Bind     Address         `json:"Bind" comment:"Bind address for this component."`
	Database DatabaseOptions `json:"Database" comment:"Database configuration for this component."`
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
