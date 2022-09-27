package config

type RoomServer struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api,omitempty"`

	Database DatabaseOptions `yaml:"database,omitempty"`
}

func (c *RoomServer) Defaults(opts DefaultOpts) {
	if !opts.Monolithic {
		c.InternalAPI.Listen = "http://localhost:7770"
		c.InternalAPI.Connect = "http://localhost:7770"
		c.Database.Defaults(20)
	}
	if opts.Generate {
		if !opts.Monolithic {
			c.Database.ConnectionString = "file:roomserver.db"
		}
	}
}

func (c *RoomServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if isMonolith { // polylith required configs below
		return
	}
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "room_server.database.connection_string", string(c.Database.ConnectionString))
	}
	checkURL(configErrs, "room_server.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "room_server.internal_ap.connect", string(c.InternalAPI.Connect))
}
