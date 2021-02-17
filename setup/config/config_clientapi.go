package config

import (
	"fmt"
	"time"
)

type ClientAPI struct {
	Matrix  *Global  `yaml:"-"`
	Derived *Derived `yaml:"-"` // TODO: Nuke Derived from orbit

	InternalAPI InternalAPIOptions `yaml:"internal_api"`
	ExternalAPI ExternalAPIOptions `yaml:"external_api"`

	// If set disables new users from registering (except via shared
	// secrets)
	RegistrationDisabled bool `yaml:"registration_disabled"`
	// If set, allows registration by anyone who also has the shared
	// secret, even if registration is otherwise disabled.
	RegistrationSharedSecret string `yaml:"registration_shared_secret"`

	// Boolean stating whether catpcha registration is enabled
	// and required
	RecaptchaEnabled bool `yaml:"enable_registration_captcha"`
	// This Home Server's ReCAPTCHA public key.
	RecaptchaPublicKey string `yaml:"recaptcha_public_key"`
	// This Home Server's ReCAPTCHA private key.
	RecaptchaPrivateKey string `yaml:"recaptcha_private_key"`
	// Secret used to bypass the captcha registration entirely
	RecaptchaBypassSecret string `yaml:"recaptcha_bypass_secret"`
	// HTTP API endpoint used to verify whether the captcha response
	// was successful
	RecaptchaSiteVerifyAPI string `yaml:"recaptcha_siteverify_api"`

	// TURN options
	TURN TURN `yaml:"turn"`

	// Rate-limiting options
	RateLimiting RateLimiting `yaml:"rate_limiting"`

	MSCs *MSCs `yaml:"mscs"`
}

func (c *ClientAPI) Defaults() {
	c.InternalAPI.Listen = "http://localhost:7771"
	c.InternalAPI.Connect = "http://localhost:7771"
	c.ExternalAPI.Listen = "http://[::]:8071"
	c.RegistrationSharedSecret = ""
	c.RecaptchaPublicKey = ""
	c.RecaptchaPrivateKey = ""
	c.RecaptchaEnabled = false
	c.RecaptchaBypassSecret = ""
	c.RecaptchaSiteVerifyAPI = ""
	c.RegistrationDisabled = false
	c.RateLimiting.Defaults()
}

func (c *ClientAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkURL(configErrs, "client_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "client_api.internal_api.connect", string(c.InternalAPI.Connect))
	if !isMonolith {
		checkURL(configErrs, "client_api.external_api.listen", string(c.ExternalAPI.Listen))
	}
	if c.RecaptchaEnabled {
		checkNotEmpty(configErrs, "client_api.recaptcha_public_key", string(c.RecaptchaPublicKey))
		checkNotEmpty(configErrs, "client_api.recaptcha_private_key", string(c.RecaptchaPrivateKey))
		checkNotEmpty(configErrs, "client_api.recaptcha_siteverify_api", string(c.RecaptchaSiteVerifyAPI))
	}
	c.TURN.Verify(configErrs)
	c.RateLimiting.Verify(configErrs)
}

type TURN struct {
	// TODO Guest Support
	// Whether or not guests can request TURN credentials
	// AllowGuests bool `yaml:"turn_allow_guests"`
	// How long the authorization should last
	UserLifetime string `yaml:"turn_user_lifetime"`
	// The list of TURN URIs to pass to clients
	URIs []string `yaml:"turn_uris"`

	// Authorization via Shared Secret
	// The shared secret from coturn
	SharedSecret string `yaml:"turn_shared_secret"`

	// Authorization via Static Username & Password
	// Hardcoded Username and Password
	Username string `yaml:"turn_username"`
	Password string `yaml:"turn_password"`
}

func (c *TURN) Verify(configErrs *ConfigErrors) {
	value := c.UserLifetime
	if value != "" {
		if _, err := time.ParseDuration(value); err != nil {
			configErrs.Add(fmt.Sprintf("invalid duration for config key %q: %s", "client_api.turn.turn_user_lifetime", value))
		}
	}
}

type RateLimiting struct {
	// Is rate limiting enabled or disabled?
	Enabled bool `yaml:"enabled"`

	// How many "slots" a user can occupy sending requests to a rate-limited
	// endpoint before we apply rate-limiting
	Threshold int64 `yaml:"threshold"`

	// The cooloff period in milliseconds after a request before the "slot"
	// is freed again
	CooloffMS int64 `yaml:"cooloff_ms"`
}

func (r *RateLimiting) Verify(configErrs *ConfigErrors) {
	if r.Enabled {
		checkPositive(configErrs, "client_api.rate_limiting.threshold", r.Threshold)
		checkPositive(configErrs, "client_api.rate_limiting.cooloff_ms", r.CooloffMS)
	}
}

func (r *RateLimiting) Defaults() {
	r.Enabled = true
	r.Threshold = 5
	r.CooloffMS = 500
}
