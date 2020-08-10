package config

import (
	"fmt"
	"time"
)

type ClientAPI struct {
	Matrix  *Global  `json:"-"`
	Derived *Derived `json:"-"` // TODO: Nuke Derived from orbit

	Listen                   Address `json:"listen" comment:"The listen address for this component."`
	Bind                     Address `json:"bind" comment:"The bind address for this component."`
	RegistrationDisabled     bool    `json:"RegistrationDisabled" comment:"Prevent new users from registering, except when using the shared secret from the\nRegistrationSharedSecret option below."`
	RegistrationSharedSecret string  `json:"RegistrationSharedSecret" comment:"If set, allows registration by anyone who knows the shared secret, even if\nregistration is otherwise disabled."`
	RecaptchaEnabled         bool    `json:"RecaptchaEnabled" comment:"Whether to require ReCAPTCHA for registration."`
	RecaptchaPublicKey       string  `json:"RecaptchaPublicKey" comment:"This server's ReCAPTCHA public key."`
	RecaptchaPrivateKey      string  `json:"RecaptchaPrivateKey" comment:"This server's ReCAPTCHA private key."`
	RecaptchaBypassSecret    string  `json:"RecaptchaBypassSecret" comment:"Secret used to bypass ReCAPTCHA entirely."`
	RecaptchaSiteVerifyAPI   string  `json:"RecaptchaSiteVerifyAPI" comment:"The URL to use for verifying if the ReCAPTCHA response was successful."`
	TURN                     TURN    `json:"TURN"`
}

func (c *ClientAPI) Defaults() {
	c.Listen = "localhost:7771"
	c.Bind = "localhost:7771"
	c.RegistrationSharedSecret = ""
	c.RecaptchaPublicKey = ""
	c.RecaptchaPrivateKey = ""
	c.RecaptchaEnabled = false
	c.RecaptchaBypassSecret = ""
	c.RecaptchaSiteVerifyAPI = ""
	c.RegistrationDisabled = false
}

func (c *ClientAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "client_api.listen", string(c.Listen))
	checkNotEmpty(configErrs, "client_api.bind", string(c.Bind))
	if c.RecaptchaEnabled {
		checkNotEmpty(configErrs, "client_api.recaptcha_public_key", string(c.RecaptchaPublicKey))
		checkNotEmpty(configErrs, "client_api.recaptcha_private_key", string(c.RecaptchaPrivateKey))
		checkNotEmpty(configErrs, "client_api.recaptcha_siteverify_api", string(c.RecaptchaSiteVerifyAPI))
	}
	c.TURN.Verify(configErrs)
}

type TURN struct {
	UserLifetime string   `json:"UserLifetime" comment:"How long the TURN authorisation should last."`
	URIs         []string `json:"URIs" comment:"The list of TURN URIs to pass to clients."`
	SharedSecret string   `json:"SharedSecret" comment:"Authorisation shared secret from coturn."`
	Username     string   `json:"Username" comment:"Authorisation static username."`
	Password     string   `json:"Password" comment:"Authorisation static password."`
	// TODO Guest Support
	// AllowGuests bool `json:"AllowGuests" comment:"Whether or not guests can request TURN credentials."`
}

func (c *TURN) Verify(configErrs *ConfigErrors) {
	value := c.UserLifetime
	if value != "" {
		if _, err := time.ParseDuration(value); err != nil {
			configErrs.Add(fmt.Sprintf("invalid duration for config key %q: %s", "client_api.turn.turn_user_lifetime", value))
		}
	}
}
