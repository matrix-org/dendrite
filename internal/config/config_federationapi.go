package config

import "github.com/matrix-org/gomatrixserverlib"

type FederationAPI struct {
	Matrix *Global `json:"-"`

	Listen                     Address                            `json:"Listen" comment:"Listen address for this component."`
	Bind                       Address                            `json:"Bind" comment:"Bind address for this component."`
	FederationCertificatePaths []Path                             `json:"FederationCertificates" comment:"List of paths to X.509 certificates to be used by the external federation listeners.\nThese certificates will be used to calculate the TLS fingerprints and other servers\nwill expect the certificate to match these fingerprints. Certificates must be in PEM\nformat."`
	TLSFingerPrints            []gomatrixserverlib.TLSFingerprint `json:"-"`
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
