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

	Login Login `yaml:"login"`

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
	c.Login.SSO.Enabled = false
}

func (c *ClientAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	c.Login.Verify(configErrs)
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

type Login struct {
	SSO SSO `yaml:"sso"`
}

// LoginTokenEnabled returns whether any login type uses
// authtypes.LoginTypeToken.
func (l *Login) LoginTokenEnabled() bool {
	return l.SSO.Enabled
}

func (l *Login) Verify(configErrs *ConfigErrors) {
	l.SSO.Verify(configErrs)
}

type SSO struct {
	// Enabled determines whether SSO should be allowed.
	Enabled bool `yaml:"enabled"`

	// CallbackURL is the absolute URL where a user agent can reach
	// the Dendrite `/_matrix/v3/login/sso/callback` endpoint. This is
	// used to create SSO redirect URLs passed to identity
	// providers. If this is empty, a default is inferred from request
	// headers. When Dendrite is running behind a proxy, this may not
	// always be the right information.
	CallbackURL string `yaml:"callback_url"`

	// Providers list the identity providers this server is capable of confirming an
	// identity with.
	Providers []IdentityProvider `yaml:"providers"`

	// DefaultProviderID is the provider to use when the client doesn't indicate one.
	// This is legacy support. If empty, the first provider listed is used.
	DefaultProviderID string `yaml:"default_provider"`
}

func (sso *SSO) Verify(configErrs *ConfigErrors) {
	var foundDefaultProvider bool
	seenPIDs := make(map[string]bool, len(sso.Providers))
	for _, p := range sso.Providers {
		p = p.WithDefaults()
		p.verifyNormalized(configErrs)
		if p.ID == sso.DefaultProviderID {
			foundDefaultProvider = true
		}
		if seenPIDs[p.ID] {
			configErrs.Add(fmt.Sprintf("duplicate identity provider for config key %q: %s", "client_api.sso.providers", p.ID))
		}
		seenPIDs[p.ID] = true
	}
	if sso.DefaultProviderID != "" && !foundDefaultProvider {
		configErrs.Add(fmt.Sprintf("identity provider ID not found for config key %q: %s", "client_api.sso.default_provider", sso.DefaultProviderID))
	}

	if sso.Enabled {
		if len(sso.Providers) == 0 {
			configErrs.Add(fmt.Sprintf("empty list for config key %q", "client_api.sso.providers"))
		}
	}
}

type IdentityProvider struct {
	// OAuth2 contains settings for IdPs based on OAuth2 (but not OpenID Connect).
	OAuth2 OAuth2 `yaml:"oauth2"`

	// OIDC contains settings for IdPs based on OpenID Connect.
	OIDC OIDC `yaml:"oidc"`

	// ID is the unique identifier of this IdP. If empty, the brand will be used.
	ID string `yaml:"id"`

	// Name is a human-friendly name of the provider. If empty, a default based on
	// the brand will be used.
	Name string `yaml:"name"`

	// Brand is a hint on how to display the IdP to the user.
	//
	// See https://github.com/matrix-org/matrix-doc/blob/old_master/informal/idp-brands.md.
	Brand SSOBrand `yaml:"brand"`

	// Icon is an MXC URI describing how to display the IdP to the user. Prefer using `brand`.
	Icon string `yaml:"icon"`

	// Type describes how this IdP is implemented. If this is empty, a default is chosen
	// based on brand or which subkeys exist.
	Type IdentityProviderType `yaml:"type"`
}

func (idp *IdentityProvider) WithDefaults() IdentityProvider {
	p := *idp
	if p.ID == "" {
		p.ID = string(p.Brand)
	}
	if p.OIDC.DiscoveryURL == "" {
		p.OIDC.DiscoveryURL = oidcDefaultDiscoveryURLs[idp.Brand]
	}
	if p.Type == "" {
		if p.OIDC.DiscoveryURL != "" {
			p.Type = SSOTypeOIDC
		} else if p.Brand == SSOBrandGitHub {
			p.Type = SSOTypeGitHub
		}
	}
	if p.Name == "" {
		p.Name = oidcDefaultNames[p.Brand]
	}

	return p
}

type OAuth2 struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
}

type OIDC struct {
	OAuth2       `yaml:",inline"`
	DiscoveryURL string `yaml:"discovery_url"`
}

func (idp *IdentityProvider) Verify(configErrs *ConfigErrors) {
	p := idp.WithDefaults()
	p.verifyNormalized(configErrs)
}

func (idp *IdentityProvider) verifyNormalized(configErrs *ConfigErrors) {
	checkNotEmpty(configErrs, "client_api.sso.providers.id", idp.ID)
	checkNotEmpty(configErrs, "client_api.sso.providers.name", idp.Name)
	if idp.Brand != "" && !checkIdentityProviderBrand(idp.Brand) {
		configErrs.Add(fmt.Sprintf("unrecognised brand in identity provider %q for config key %q: %s", idp.ID, "client_api.sso.providers", idp.Brand))
	}
	if idp.Icon != "" {
		checkIconURL(configErrs, "client_api.sso.providers.icon", idp.Icon)
	}

	switch idp.Type {
	case SSOTypeOIDC:
		checkNotEmpty(configErrs, "client_api.sso.providers.oidc.client_id", idp.OIDC.ClientID)
		checkNotEmpty(configErrs, "client_api.sso.providers.oidc.client_secret", idp.OIDC.ClientSecret)
		checkNotEmpty(configErrs, "client_api.sso.providers.oidc.discovery_url", idp.OIDC.DiscoveryURL)

		checkEmpty(configErrs, "client_api.sso.providers.oauth2.client_id", idp.OAuth2.ClientID)
		checkEmpty(configErrs, "client_api.sso.providers.oauth2.client_secret", idp.OAuth2.ClientSecret)

	case SSOTypeGitHub:
		checkNotEmpty(configErrs, "client_api.sso.providers.oauth2.client_id", idp.OAuth2.ClientID)
		checkNotEmpty(configErrs, "client_api.sso.providers.oauth2.client_secret", idp.OAuth2.ClientSecret)

	default:
		configErrs.Add(fmt.Sprintf("unrecognised type in identity provider %q for config key %q: %s", idp.ID, "client_api.sso.providers", idp.Type))
	}
}

// See https://github.com/matrix-org/matrix-doc/blob/old_master/informal/idp-brands.md.
func checkIdentityProviderBrand(s SSOBrand) bool {
	switch s {
	case SSOBrandApple, SSOBrandFacebook, SSOBrandGitHub, SSOBrandGitLab, SSOBrandGoogle, SSOBrandTwitter:
		return true
	default:
		return false
	}
}

// SSOBrand corresponds to https://github.com/matrix-org/matrix-spec-proposals/blob/old_master/informal/idp-brands.md
type SSOBrand string

const (
	SSOBrandApple    SSOBrand = "apple"
	SSOBrandFacebook SSOBrand = "facebook"
	SSOBrandGitHub   SSOBrand = "github"
	SSOBrandGitLab   SSOBrand = "gitlab"
	SSOBrandGoogle   SSOBrand = "google"
	SSOBrandTwitter  SSOBrand = "twitter"
)

var (
	oidcDefaultDiscoveryURLs = map[SSOBrand]string{
		// https://developers.facebook.com/docs/facebook-login/limited-login/token/
		SSOBrandFacebook: "https://www.facebook.com/.well-known/openid-configuration/",
		// https://docs.gitlab.com/ee/integration/openid_connect_provider.html
		SSOBrandGitLab: "https://gitlab.com/.well-known/openid-configuration",
		// https://developers.google.com/identity/protocols/oauth2/openid-connect
		SSOBrandGoogle: "https://accounts.google.com/.well-known/openid-configuration",
	}
	oidcDefaultNames = map[SSOBrand]string{
		SSOBrandApple:    "Apple",
		SSOBrandFacebook: "Facebook",
		SSOBrandGitHub:   "GitHub",
		SSOBrandGitLab:   "GitLab",
		SSOBrandGoogle:   "Google",
		SSOBrandTwitter:  "Twitter",
	}
)

type IdentityProviderType string

const (
	SSOTypeOIDC   IdentityProviderType = "oidc"
	SSOTypeGitHub IdentityProviderType = "github"
)

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
