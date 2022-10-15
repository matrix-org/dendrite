package config

type Monolith struct {
	HTTPBindAddr  HTTPAddress `yaml:"http_bind_address"`
	HTTPSBindAddr HTTPAddress `yaml:"https_bind_address"`

	TlsCertificatePath Path `yaml:"tls_cert_path"`
	TlsPrivateKeyPath  Path `yaml:"tls_key_path"`
}

func (c *Monolith) Defaults(opts DefaultOpts) {
	if !opts.Monolithic {
		return
	}
	c.HTTPBindAddr = "http://:8008"
	c.HTTPSBindAddr = "https://:8448"
}

func (c *Monolith) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if !isMonolith {
		return
	}
	checkURL(configErrs, "monolith.http_bind_address", string(c.HTTPBindAddr))
	checkURL(configErrs, "monolith.https_bind_address", string(c.HTTPSBindAddr))
}
