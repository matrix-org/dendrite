package config

type SyncAPI struct {
	Matrix *Global `json:"-"`

	Listen   Address         `json:"Listen" comment:"Listen address for this component."`
	Bind     Address         `json:"Bind" comment:"Bind address for this component."`
	Database DatabaseOptions `json:"Database" comment:"Database configuration for this component."`
}

func (c *SyncAPI) Defaults() {
	c.Listen = "localhost:7773"
	c.Bind = "localhost:7773"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:syncapi.db"
}

func (c *SyncAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "SyncAPI.Listen", string(c.Listen))
	checkNotEmpty(configErrs, "SyncAPI.Bind", string(c.Bind))
	checkNotEmpty(configErrs, "SyncAPI.Database.ConnectionString", string(c.Database.ConnectionString))
}
