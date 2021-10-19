package config

type PushServer struct {
	Matrix *Global `yaml:"-"`

	// DisableTLSValidation disables the validation of X.509 TLS certs
	// on remote Push gateway endpoints. This is not recommended in
	// production!
	DisableTLSValidation bool `yaml:"disable_tls_validation"`
}

func (c *PushServer) Defaults(generate bool) {
}

func (c *PushServer) Verify(configErrs *ConfigErrors, isMonolith bool) {
}
