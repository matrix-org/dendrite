package config

type SyncAPI struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	Database        DataSource      `yaml:"database"`
	DatabaseOptions DatabaseOptions `yaml:"database_options"`
}

func (c *SyncAPI) Defaults() {
	c.Listen = "localhost:7773"
	c.Bind = "localhost:7773"
	c.Database = "file:syncapi.db"
	c.DatabaseOptions.Defaults()
}

func (c *SyncAPI) Verify(configErrs *configErrors) {
	checkNotEmpty(configErrs, "sync_api.listen", string(c.Listen))
	checkNotEmpty(configErrs, "sync_api.bind", string(c.Bind))
	checkNotEmpty(configErrs, "sync_api.database", string(c.Database))
}
