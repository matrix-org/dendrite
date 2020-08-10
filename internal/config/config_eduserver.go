package config

type EDUServer struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`
}

func (c *EDUServer) Defaults() {
	c.Listen = "localhost:7778"
	c.Bind = "localhost:7778"
}

func (c *EDUServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "edu_server.listen", string(c.Listen))
	checkNotEmpty(configErrs, "edu_server.bind", string(c.Bind))
}
