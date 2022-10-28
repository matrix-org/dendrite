package config

import (
	"fmt"
	"time"
)

type ClientAPI struct {
	Matrix  *Global  `yaml:"-"`
	Derived *Derived `yaml:"-"` // TODO: Nuke Derived from orbit

	InternalAPI InternalAPIOptions `yaml:"internal_api,omitempty"`
	ExternalAPI ExternalAPIOptions `yaml:"external_api,omitempty"`

	// If set disables new users from registering (except via shared
	// secrets)
	RegistrationDisabled bool `yaml:"registration_disabled"`

	// Enable registration without captcha verification or shared secret.
	// This option is populated by the -really-enable-open-registration
	// command line parameter as it is not recommended.
	OpenRegistrationWithoutVerificationEnabled bool `yaml:"-"`

	// If set, allows registration by anyone who also has the shared
	// secret, even if registration is otherwise disabled.
	RegistrationSharedSecret string `yaml:"registration_shared_secret"`
	// If set, prevents guest accounts from being created. Only takes
	// effect if registration is enabled, otherwise guests registration
	// is forbidden either way.
	GuestsDisabled bool `yaml:"guests_disabled"`

	// Boolean stating whether catpcha registration is enabled
	// and required
	RecaptchaEnabled bool `yaml:"enable_registration_captcha"`
	// Recaptcha api.js Url, for compatible with hcaptcha.com, etc.
	RecaptchaApiJsUrl string `yaml:"recaptcha_api_js_url"`
	// Recaptcha div class for sitekey, for compatible with hcaptcha.com, etc.
	RecaptchaSitekeyClass string `yaml:"recaptcha_sitekey_class"`
	// Recaptcha form field, for compatible with hcaptcha.com, etc.
	RecaptchaFormField string `yaml:"recaptcha_form_field"`
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

	MSCs *MSCs `yaml:"-"`
}

func (c *ClientAPI) Defaults(opts DefaultOpts) {
	if !opts.Monolithic {
		c.InternalAPI.Listen = "http://localhost:7771"
		c.InternalAPI.Connect = "http://localhost:7771"
		c.ExternalAPI.Listen = "http://[::]:8071"
	}
	c.RegistrationSharedSecret = ""
	c.RecaptchaPublicKey = ""
	c.RecaptchaPrivateKey = ""
	c.RecaptchaEnabled = false
	c.RecaptchaBypassSecret = ""
	c.RecaptchaSiteVerifyAPI = ""
	c.RegistrationDisabled = true
	c.OpenRegistrationWithoutVerificationEnabled = false
	c.RateLimiting.Defaults()
}

func (c *ClientAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	c.TURN.Verify(configErrs)
	c.RateLimiting.Verify(configErrs)
	if c.RecaptchaEnabled {
		checkNotEmpty(configErrs, "client_api.recaptcha_public_key", c.RecaptchaPublicKey)
		checkNotEmpty(configErrs, "client_api.recaptcha_private_key", c.RecaptchaPrivateKey)
		checkNotEmpty(configErrs, "client_api.recaptcha_siteverify_api", c.RecaptchaSiteVerifyAPI)
		if c.RecaptchaSiteVerifyAPI == "" {
			c.RecaptchaSiteVerifyAPI = "https://www.google.com/recaptcha/api/siteverify"
		}
		if c.RecaptchaApiJsUrl == "" {
			c.RecaptchaApiJsUrl = "https://www.google.com/recaptcha/api.js"
		}
		if c.RecaptchaFormField == "" {
			c.RecaptchaFormField = "g-recaptcha"
		}
		if c.RecaptchaSitekeyClass == "" {
			c.RecaptchaSitekeyClass = "g-recaptcha-response"
		}
	}
	// Ensure there is any spam counter measure when enabling registration
	if !c.RegistrationDisabled && !c.OpenRegistrationWithoutVerificationEnabled {
		if !c.RecaptchaEnabled {
			configErrs.Add(
				"You have tried to enable open registration without any secondary verification methods " +
					"(such as reCAPTCHA). By enabling open registration, you are SIGNIFICANTLY " +
					"increasing the risk that your server will be used to send spam or abuse, and may result in " +
					"your server being banned from some rooms. If you are ABSOLUTELY CERTAIN you want to do this, " +
					"start Dendrite with the -really-enable-open-registration command line flag. Otherwise, you " +
					"should set the registration_disabled option in your Dendrite config.",
			)
		}
	}
	if isMonolith { // polylith required configs below
		return
	}
	checkURL(configErrs, "client_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "client_api.internal_api.connect", string(c.InternalAPI.Connect))
	checkURL(configErrs, "client_api.external_api.listen", string(c.ExternalAPI.Listen))
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

	// A list of users that are exempt from rate limiting, i.e. if you want
	// to run Mjolnir or other bots.
	ExemptUserIDs []string `yaml:"exempt_user_ids"`
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
