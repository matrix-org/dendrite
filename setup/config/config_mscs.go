package config

type MSCs struct {
	Matrix *Global `yaml:"-"`

	// The MSCs to enable. Supported MSCs include:
	// 'msc2444': Peeking over federation - https://github.com/matrix-org/matrix-doc/pull/2444
	// 'msc2753': Peeking via /sync - https://github.com/matrix-org/matrix-doc/pull/2753
	// 'msc2836': Threading - https://github.com/matrix-org/matrix-doc/pull/2836
	// 'msc2946': Spaces Summary - https://github.com/matrix-org/matrix-doc/pull/2946
	MSCs []string `yaml:"mscs"`

	Database DatabaseOptions `yaml:"database"`
}

func (c *MSCs) Defaults() {
	c.Database.Defaults()
	c.Database.ConnectionString = "file:mscs.db"
}

// Enabled returns true if the given msc is enabled. Should in the form 'msc12345'.
func (c *MSCs) Enabled(msc string) bool {
	for _, m := range c.MSCs {
		if m == msc {
			return true
		}
	}
	return false
}

func (c *MSCs) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "mscs.database.connection_string", string(c.Database.ConnectionString))
}
