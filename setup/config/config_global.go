package config

import (
	"fmt"
	"html/template"
	"math/rand"
	"path/filepath"
	textTemplate "text/template"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/ed25519"
)

type Global struct {
	// The name of the server. This is usually the domain name, e.g 'matrix.org', 'localhost'.
	ServerName gomatrixserverlib.ServerName `yaml:"server_name"`

	// Path to the private key which will be used to sign requests and events.
	PrivateKeyPath Path `yaml:"private_key"`

	// The private key which will be used to sign requests and events.
	PrivateKey ed25519.PrivateKey `yaml:"-"`

	// An arbitrary string used to uniquely identify the PrivateKey. Must start with the
	// prefix "ed25519:".
	KeyID gomatrixserverlib.KeyID `yaml:"-"`

	// Information about old private keys that used to be used to sign requests and
	// events on this domain. They will not be used but will be advertised to other
	// servers that ask for them to help verify old events.
	OldVerifyKeys []OldVerifyKeys `yaml:"old_private_keys"`

	// How long a remote server can cache our server key for before requesting it again.
	// Increasing this number will reduce the number of requests made by remote servers
	// for our key, but increases the period a compromised key will be considered valid
	// by remote servers.
	// Defaults to 24 hours.
	KeyValidityPeriod time.Duration `yaml:"key_validity_period"`

	// Global pool of database connections, which is used only in monolith mode. If a
	// component does not specify any database options of its own, then this pool of
	// connections will be used instead. This way we don't have to manage connection
	// counts on a per-component basis, but can instead do it for the entire monolith.
	// In a polylith deployment, this will be ignored.
	DatabaseOptions DatabaseOptions `yaml:"database"`

	// The server name to delegate server-server communications to, with optional port
	WellKnownServerName string `yaml:"well_known_server_name"`

	// Disables federation. Dendrite will not be able to make any outbound HTTP requests
	// to other servers and the federation API will not be exposed.
	DisableFederation bool `yaml:"disable_federation"`

	// Configures the handling of presence events.
	Presence PresenceOptions `yaml:"presence"`

	// List of domains that the server will trust as identity servers to
	// verify third-party identifiers.
	// Defaults to an empty array.
	TrustedIDServers []string `yaml:"trusted_third_party_id_servers"`

	// JetStream configuration
	JetStream JetStream `yaml:"jetstream"`

	// Metrics configuration
	Metrics Metrics `yaml:"metrics"`

	// Sentry configuration
	Sentry Sentry `yaml:"sentry"`

	// DNS caching options for all outbound HTTP requests
	DNSCache DNSCacheOptions `yaml:"dns_cache"`

	// ServerNotices configuration used for sending server notices
	ServerNotices ServerNotices `yaml:"server_notices"`

	// Consent tracking options
	UserConsentOptions UserConsentOptions `yaml:"user_consent"`
}

func (c *Global) Defaults(generate bool) {
	if generate {
		c.ServerName = "localhost"
		c.PrivateKeyPath = "matrix_key.pem"
		_, c.PrivateKey, _ = ed25519.GenerateKey(rand.New(rand.NewSource(0)))
		c.KeyID = "ed25519:auto"
	}
	c.KeyValidityPeriod = time.Hour * 24 * 7

	c.JetStream.Defaults(generate)
	c.Metrics.Defaults(generate)
	c.DNSCache.Defaults()
	c.Sentry.Defaults()
	c.UserConsentOptions.Defaults()
	c.ServerNotices.Defaults(generate)
}

func (c *Global) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "global.server_name", string(c.ServerName))
	checkNotEmpty(configErrs, "global.private_key", string(c.PrivateKeyPath))

	c.JetStream.Verify(configErrs, isMonolith)
	c.Metrics.Verify(configErrs, isMonolith)
	c.Sentry.Verify(configErrs, isMonolith)
	c.DNSCache.Verify(configErrs, isMonolith)
	c.UserConsentOptions.Verify(configErrs, isMonolith)
	c.ServerNotices.Verify(configErrs, isMonolith)
}

type OldVerifyKeys struct {
	// Path to the private key.
	PrivateKeyPath Path `yaml:"private_key"`

	// The private key itself.
	PrivateKey ed25519.PrivateKey `yaml:"-"`

	// The key ID of the private key.
	KeyID gomatrixserverlib.KeyID `yaml:"-"`

	// When the private key was designed as "expired", as a UNIX timestamp
	// in millisecond precision.
	ExpiredAt gomatrixserverlib.Timestamp `yaml:"expired_at"`
}

// The configuration to use for Prometheus metrics
type Metrics struct {
	// Whether or not the metrics are enabled
	Enabled bool `yaml:"enabled"`
	// Use BasicAuth for Authorization
	BasicAuth struct {
		// Authorization via Static Username & Password
		// Hardcoded Username and Password
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"basic_auth"`
}

func (c *Metrics) Defaults(generate bool) {
	c.Enabled = false
	if generate {
		c.BasicAuth.Username = "metrics"
		c.BasicAuth.Password = "metrics"
	}
}

func (c *Metrics) Verify(configErrs *ConfigErrors, isMonolith bool) {
}

// ServerNotices defines the configuration used for sending server notices
type ServerNotices struct {
	Enabled bool `yaml:"enabled"`
	// The localpart to be used when sending notices
	LocalPart string `yaml:"local_part"`
	// The displayname to be used when sending notices
	DisplayName string `yaml:"display_name"`
	// The avatar of this user
	AvatarURL string `yaml:"avatar"`
	// The roomname to be used when creating messages
	RoomName string `yaml:"room_name"`
}

func (c *ServerNotices) Defaults(generate bool) {
	if generate {
		c.Enabled = true
		c.LocalPart = "_server"
		c.DisplayName = "Server Alert"
		c.RoomName = "Server Alert"
		c.AvatarURL = ""
	}
}

func (c *ServerNotices) Verify(errors *ConfigErrors, isMonolith bool) {}

// The configuration to use for Sentry error reporting
type Sentry struct {
	Enabled bool `yaml:"enabled"`
	// The DSN to connect to e.g "https://examplePublicKey@o0.ingest.sentry.io/0"
	// See https://docs.sentry.io/platforms/go/configuration/options/
	DSN string `yaml:"dsn"`
	// The environment e.g "production"
	// See https://docs.sentry.io/platforms/go/configuration/environments/
	Environment string `yaml:"environment"`
}

func (c *Sentry) Defaults() {
	c.Enabled = false
}

func (c *Sentry) Verify(configErrs *ConfigErrors, isMonolith bool) {
}

type DatabaseOptions struct {
	// The connection string, file:filename.db or postgres://server....
	ConnectionString DataSource `yaml:"connection_string"`
	// Maximum open connections to the DB (0 = use default, negative means unlimited)
	MaxOpenConnections int `yaml:"max_open_conns"`
	// Maximum idle connections to the DB (0 = use default, negative means unlimited)
	MaxIdleConnections int `yaml:"max_idle_conns"`
	// maximum amount of time (in seconds) a connection may be reused (<= 0 means unlimited)
	ConnMaxLifetimeSeconds int `yaml:"conn_max_lifetime"`
}

func (c *DatabaseOptions) Defaults(conns int) {
	c.MaxOpenConnections = conns
	c.MaxIdleConnections = 2
	c.ConnMaxLifetimeSeconds = -1
}

func (c *DatabaseOptions) Verify(configErrs *ConfigErrors, isMonolith bool) {
}

// MaxIdleConns returns maximum idle connections to the DB
func (c DatabaseOptions) MaxIdleConns() int {
	return c.MaxIdleConnections
}

// MaxOpenConns returns maximum open connections to the DB
func (c DatabaseOptions) MaxOpenConns() int {
	return c.MaxOpenConnections
}

// ConnMaxLifetime returns maximum amount of time a connection may be reused
func (c DatabaseOptions) ConnMaxLifetime() time.Duration {
	return time.Duration(c.ConnMaxLifetimeSeconds) * time.Second
}

type DNSCacheOptions struct {
	// Whether the DNS cache is enabled or not
	Enabled bool `yaml:"enabled"`
	// How many entries to store in the DNS cache at a given time
	CacheSize int `yaml:"cache_size"`
	// How long a cache entry should be considered valid for
	CacheLifetime time.Duration `yaml:"cache_lifetime"`
}

func (c *DNSCacheOptions) Defaults() {
	c.Enabled = false
	c.CacheSize = 256
	c.CacheLifetime = time.Minute * 5
}

func (c *DNSCacheOptions) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkPositive(configErrs, "cache_size", int64(c.CacheSize))
	checkPositive(configErrs, "cache_lifetime", int64(c.CacheLifetime))
}

// Consent tracking configuration
// If either require_at_registration or send_server_notice_to_guest are true, consent
// messages will be sent to the users.
type UserConsentOptions struct {
	// If consent tracking is enabled or not
	Enabled bool `yaml:"enabled"`
	// Randomly generated string to be used to calculate the HMAC
	FormSecret string `yaml:"form_secret"`
	// Require consent when user registers for the first time
	RequireAtRegistration bool `yaml:"require_at_registration"`
	// The name to be shown to the user
	PolicyName string `yaml:"policy_name"`
	// The directory to search for *.gohtml templates
	TemplateDir string `yaml:"template_dir"`
	// The version of the policy. When loading templates, ".gohtml" template is added as a suffix
	// e.g: ${template_dir}/1.0.gohtml needs to exist, if this is set to 1.0
	Version string `yaml:"version"`
	// Send a consent message to guest users
	SendServerNoticeToGuest bool `yaml:"send_server_notice_to_guest"`
	// Default message to send to users
	ServerNoticeContent struct {
		MsgType string `yaml:"msg_type"`
		Body    string `yaml:"body"`
	} `yaml:"server_notice_content"`
	// The error message to display if the user hasn't given their consent yet
	BlockEventsError string `yaml:"block_events_error"`
	// All loaded templates
	Templates     *template.Template     `yaml:"-"`
	TextTemplates *textTemplate.Template `yaml:"-"`
	// The base URL this homeserver will serve clients on, e.g. https://matrix.org
	BaseURL string `yaml:"base_url"`
}

func (c *UserConsentOptions) Defaults() {
	c.Enabled = false
	c.RequireAtRegistration = false
	c.SendServerNoticeToGuest = false
	c.PolicyName = "Privacy Policy"
	c.Version = "1.0"
	c.TemplateDir = "./templates/privacy"
}

func (c *UserConsentOptions) Verify(configErrors *ConfigErrors, isMonolith bool) {
	if c.Enabled {
		checkNotEmpty(configErrors, "template_dir", c.TemplateDir)
		checkNotEmpty(configErrors, "version", c.Version)
		checkNotEmpty(configErrors, "policy_name", c.PolicyName)
		checkNotEmpty(configErrors, "form_secret", c.FormSecret)
		checkNotEmpty(configErrors, "base_url", c.BaseURL)
		if len(*configErrors) > 0 {
			return
		}

		p, err := filepath.Abs(c.TemplateDir)
		if err != nil {
			configErrors.Add("unable to get template directory")
			return
		}

		c.TextTemplates = textTemplate.Must(textTemplate.New("blockEventsError").Parse(c.BlockEventsError))
		c.TextTemplates = textTemplate.Must(c.TextTemplates.New("serverNoticeTemplate").Parse(c.ServerNoticeContent.Body))

		// Read all defined *.gohtml templates
		t, err := template.ParseGlob(filepath.Join(p, "*.gohtml"))
		if err != nil || t == nil {
			configErrors.Add(fmt.Sprintf("unable to read consent templates: %+v", err))
			return
		}
		c.Templates = t
		// Verify we've got a template for the defined version
		versionTemplate := c.Templates.Lookup(c.Version + ".gohtml")
		if versionTemplate == nil {
			configErrors.Add(fmt.Sprintf("unable to load defined '%s' policy template", c.Version))
		}
	}
}

// PresenceOptions defines possible configurations for presence events.
type PresenceOptions struct {
	// Whether inbound presence events are allowed
	EnableInbound bool `yaml:"enable_inbound"`
	// Whether outbound presence events are allowed
	EnableOutbound bool `yaml:"enable_outbound"`
}
