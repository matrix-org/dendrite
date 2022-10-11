package config

type SyncAPI struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api,omitempty"`
	ExternalAPI ExternalAPIOptions `yaml:"external_api,omitempty"`

	Database DatabaseOptions `yaml:"database,omitempty"`

	RealIPHeader string `yaml:"real_ip_header"`

	Fulltext Fulltext `yaml:"search"`
}

func (c *SyncAPI) Defaults(opts DefaultOpts) {
	if !opts.Monolithic {
		c.InternalAPI.Listen = "http://localhost:7773"
		c.InternalAPI.Connect = "http://localhost:7773"
		c.ExternalAPI.Listen = "http://localhost:8073"
		c.Database.Defaults(20)
	}
	c.Fulltext.Defaults(opts)
	if opts.Generate {
		if !opts.Monolithic {
			c.Database.ConnectionString = "file:syncapi.db"
		}
	}
}

func (c *SyncAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	c.Fulltext.Verify(configErrs, isMonolith)
	if isMonolith { // polylith required configs below
		return
	}
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "sync_api.database", string(c.Database.ConnectionString))
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

func (f *Fulltext) Defaults(opts DefaultOpts) {
	f.Enabled = false
	f.IndexPath = "./searchindex"
	f.Language = "en"
}

func (f *Fulltext) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if !f.Enabled {
		return
	}
	checkNotEmpty(configErrs, "syncapi.search.index_path", string(f.IndexPath))
	checkNotEmpty(configErrs, "syncapi.search.language", f.Language)
}
