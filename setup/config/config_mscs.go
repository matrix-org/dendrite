package config

type MSCs struct {
	Matrix *Global `yaml:"-"`

	// The MSCs to enable
	MSCs []string `yaml:"mscs"`

	Database DatabaseOptions `yaml:"database"`
}

func (c *MSCs) Defaults() {
	c.Database.Defaults()
	c.Database.ConnectionString = "file:mscs.db"
}

func (c *MSCs) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "mscs.database.connection_string", string(c.Database.ConnectionString))
}
