package config

type CurrentStateServer struct {
	Matrix *Global `json:"-"`

	Listen   Address         `json:"listen" comment:"Listen address for this component."`
	Bind     Address         `json:"bind" comment:"Bind address for this component."`
	Database DatabaseOptions `json:"Database" comment:"Database configuration for this component."`
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
