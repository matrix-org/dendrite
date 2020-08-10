package config

import "github.com/matrix-org/gomatrixserverlib"

type FederationAPI struct {
	Matrix *Global `yaml:"-"`

	Listen Address `yaml:"listen"`
	Bind   Address `yaml:"bind"`

	// List of paths to X509 certificates used by the external federation listeners.
	// These are used to calculate the TLS fingerprints to publish for this server.
	// Other matrix servers talking to this server will expect the x509 certificate
	// to match one of these certificates.
	// The certificates should be in PEM format.
	FederationCertificatePaths []Path `yaml:"federation_certificates"`

	// A list of SHA256 TLS fingerprints for the X509 certificates used by the
	// federation listener for this server.
	TLSFingerPrints []gomatrixserverlib.TLSFingerprint `yaml:"-"`
}

func (c *FederationAPI) Defaults() {
	c.Listen = "localhost:7772"
	c.Bind = "localhost:7772"
}

func (c *FederationAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "federation_api.listen", string(c.Listen))
	checkNotEmpty(configErrs, "federation_api.bind", string(c.Bind))
	// TODO: not applicable always, e.g. in demos
	//checkNotZero(configErrs, "federation_api.federation_certificates", int64(len(c.FederationCertificatePaths)))
}
