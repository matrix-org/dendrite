package config

type SyncAPI struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	Database DatabaseOptions `yaml:"database"`
}

func (c *SyncAPI) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7773"
	c.InternalAPI.Connect = "http://localhost:7773"
	c.Database.Defaults()
	c.Database.ConnectionString = "file:syncapi.db"
}

func (c *SyncAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "sync_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "sync_api.internal_api.bind", string(c.InternalAPI.Connect))
	checkNotEmpty(configErrs, "sync_api.database", string(c.Database.ConnectionString))
}
