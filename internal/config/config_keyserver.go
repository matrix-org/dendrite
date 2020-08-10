package config

type KeyServer struct {
	Matrix *Global `json:"-"`

	Listen   Address         `json:"Listen" comment:"Listen address for this component."`
	Bind     Address         `json:"Bind" comment:"Bind address for this component."`
	Database DatabaseOptions `json:"Database" comment:"Database configuration for this component."`
}

func (c *KeyServer) Defaults() {
	c.Listen = "localhost:7779"
	c.Bind = "localhost:7779"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:keyserver.db"
}

func (c *KeyServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "key_server.listen", string(c.Listen))
	checkNotEmpty(configErrs, "key_server.bind", string(c.Bind))
	checkNotEmpty(configErrs, "key_server.database.connection_string", string(c.Database.ConnectionString))
}
