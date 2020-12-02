package config

type FederationAPI struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api"`
	ExternalAPI ExternalAPIOptions `yaml:"external_api"`

	// List of paths to X509 certificates used by the external federation listeners.
	// These are used to calculate the TLS fingerprints to publish for this server.
	// Other matrix servers talking to this server will expect the x509 certificate
	// to match one of these certificates.
	// The certificates should be in PEM format.
	FederationCertificatePaths []Path `yaml:"federation_certificates"`
}

func (c *FederationAPI) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7772"
	c.InternalAPI.Connect = "http://localhost:7772"
	c.ExternalAPI.Listen = "http://[::]:8072"
}

func (c *FederationAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "federation_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "federation_api.internal_api.connect", string(c.InternalAPI.Connect))
	if !isMonolith {
		checkURL(configErrs, "federation_api.external_api.listen", string(c.ExternalAPI.Listen))
	}
	// TODO: not applicable always, e.g. in demos
	//checkNotZero(configErrs, "federation_api.federation_certificates", int64(len(c.FederationCertificatePaths)))
}
