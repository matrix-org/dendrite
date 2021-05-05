package config

type PushServer struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`

	Database DatabaseOptions `yaml:"database"`
}

func (c *PushServer) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7783"
	c.InternalAPI.Connect = "http://localhost:7783"
	c.Database.Defaults(10)
	c.Database.ConnectionString = "file:pushserver.db"
}

func (c *PushServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "room_server.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "room_server.internal_ap.bind", string(c.InternalAPI.Connect))
	checkNotEmpty(configErrs, "room_server.database.connection_string", string(c.Database.ConnectionString))
}
