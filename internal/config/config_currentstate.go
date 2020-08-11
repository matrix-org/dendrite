package config

type CurrentStateServer struct {
	Matrix *Global `json:"-"`

	Listen   Address         `json:"Listen" comment:"Listen address for this component."`
	Bind     Address         `json:"Bind" comment:"Bind address for this component."`
	Database DatabaseOptions `json:"Database" comment:"Database configuration for this component."`
}

func (c *CurrentStateServer) Defaults() {
	c.Listen = "localhost:7782"
	c.Bind = "localhost:7782"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:currentstate.db"
}

func (c *CurrentStateServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "CurrentStateServer.Listen", string(c.Listen))
	checkNotEmpty(configErrs, "CurrentStateServer.Bind", string(c.Bind))
	checkNotEmpty(configErrs, "CurrentStateServer.Database.ConnectionString", string(c.Database.ConnectionString))
}
