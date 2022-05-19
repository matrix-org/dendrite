package config

type SyncAPI struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`
	ExternalAPI ExternalAPIOptions `yaml:"external_api"`

	Database DatabaseOptions `yaml:"database"`

	RealIPHeader string `yaml:"real_ip_header"`

	Fulltext Fulltext `yaml:"fulltext"`
}

func (c *SyncAPI) Defaults(generate bool) {
	c.InternalAPI.Listen = "http://localhost:7773"
	c.InternalAPI.Connect = "http://localhost:7773"
	c.ExternalAPI.Listen = "http://localhost:8073"
	c.Database.Defaults(10)
	c.Fulltext.Defaults(generate)
	if generate {
		c.Database.ConnectionString = "file:syncapi.db"
	}
}

func (c *SyncAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	c.Fulltext.Verify(configErrs, isMonolith)
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "sync_api.database", string(c.Database.ConnectionString))
	}
	if isMonolith { // polylith required configs below
		return
	}
	checkURL(configErrs, "sync_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "sync_api.internal_api.connect", string(c.InternalAPI.Connect))
	checkURL(configErrs, "sync_api.external_api.listen", string(c.ExternalAPI.Listen))
}

type Fulltext struct {
	Enabled   bool   `yaml:"enabled"`
	IndexPath Path   `yaml:"index_path"`
	InMemory  bool   `yaml:"in_memory"` // only useful in tests
	Language  string `yaml:"language"`  // the language to use when analysing content
}

func (f *Fulltext) Defaults(generate bool) {
	f.Enabled = false
	f.IndexPath = "./fulltextindex"
	f.Language = "en"
	if generate {
		f.Enabled = true
		f.InMemory = true
	}
}

func (f *Fulltext) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "syncapi.fulltext.index_path", string(f.IndexPath))
	checkNotEmpty(configErrs, "syncapi.fulltext.language", f.Language)
}
