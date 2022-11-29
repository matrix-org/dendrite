package config

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/ed25519"
)

type Global struct {
	// Signing identity contains the server name, private key and key ID of
	// the deployment.
	gomatrixserverlib.SigningIdentity `yaml:",inline"`

	// The secondary server names, used for virtual hosting.
	VirtualHosts []*VirtualHost `yaml:"-"`

	// Path to the private key which will be used to sign requests and events.
	PrivateKeyPath Path `yaml:"private_key"`

	// Information about old private keys that used to be used to sign requests and
	// events on this domain. They will not be used but will be advertised to other
	// servers that ask for them to help verify old events.
	OldVerifyKeys []*OldVerifyKeys `yaml:"old_private_keys"`

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
	DatabaseOptions DatabaseOptions `yaml:"database,omitempty"`

	// The server name to delegate server-server communications to, with optional port
	WellKnownServerName string `yaml:"well_known_server_name"`

	// The server name to delegate client-server communications to, with optional port
	WellKnownClientName string `yaml:"well_known_client_name"`

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

	// ReportStats configures opt-in phone-home statistics reporting.
	ReportStats ReportStats `yaml:"report_stats"`

	// Configuration for the caches.
	Cache Cache `yaml:"cache"`
}

func (c *Global) Defaults(opts DefaultOpts) {
	if opts.Generate {
		c.ServerName = "localhost"
		c.PrivateKeyPath = "matrix_key.pem"
		_, c.PrivateKey, _ = ed25519.GenerateKey(rand.New(rand.NewSource(0)))
		c.KeyID = "ed25519:auto"
		c.TrustedIDServers = []string{
			"matrix.org",
			"vector.im",
		}
	}
	c.KeyValidityPeriod = time.Hour * 24 * 7
	if opts.Monolithic {
		c.DatabaseOptions.Defaults(90)
	}
	c.JetStream.Defaults(opts)
	c.Metrics.Defaults(opts)
	c.DNSCache.Defaults()
	c.Sentry.Defaults()
	c.ServerNotices.Defaults(opts)
	c.ReportStats.Defaults()
	c.Cache.Defaults()
}

func (c *Global) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "global.server_name", string(c.ServerName))
	checkNotEmpty(configErrs, "global.private_key", string(c.PrivateKeyPath))

	for _, v := range c.VirtualHosts {
		v.Verify(configErrs)
	}

	c.JetStream.Verify(configErrs, isMonolith)
	c.Metrics.Verify(configErrs, isMonolith)
	c.Sentry.Verify(configErrs, isMonolith)
	c.DNSCache.Verify(configErrs, isMonolith)
	c.ServerNotices.Verify(configErrs, isMonolith)
	c.ReportStats.Verify(configErrs, isMonolith)
	c.Cache.Verify(configErrs, isMonolith)
}

func (c *Global) IsLocalServerName(serverName gomatrixserverlib.ServerName) bool {
	if c.ServerName == serverName {
		return true
	}
	for _, v := range c.VirtualHosts {
		if v.ServerName == serverName {
			return true
		}
	}
	return false
}

func (c *Global) SplitLocalID(sigil byte, id string) (string, gomatrixserverlib.ServerName, error) {
	u, s, err := gomatrixserverlib.SplitID(sigil, id)
	if err != nil {
		return u, s, err
	}
	if !c.IsLocalServerName(s) {
		return u, s, fmt.Errorf("server name %q not known", s)
	}
	return u, s, nil
}

func (c *Global) VirtualHost(serverName gomatrixserverlib.ServerName) *VirtualHost {
	for _, v := range c.VirtualHosts {
		if v.ServerName == serverName {
			return v
		}
	}
	return nil
}

func (c *Global) VirtualHostForHTTPHost(serverName gomatrixserverlib.ServerName) *VirtualHost {
	for _, v := range c.VirtualHosts {
		if v.ServerName == serverName {
			return v
		}
		for _, h := range v.MatchHTTPHosts {
			if h == serverName {
				return v
			}
		}
	}
	return nil
}

func (c *Global) SigningIdentityFor(serverName gomatrixserverlib.ServerName) (*gomatrixserverlib.SigningIdentity, error) {
	for _, id := range c.SigningIdentities() {
		if id.ServerName == serverName {
			return id, nil
		}
	}
	return nil, fmt.Errorf("no signing identity %q", serverName)
}

func (c *Global) SigningIdentities() []*gomatrixserverlib.SigningIdentity {
	identities := make([]*gomatrixserverlib.SigningIdentity, 0, len(c.VirtualHosts)+1)
	identities = append(identities, &c.SigningIdentity)
	for _, v := range c.VirtualHosts {
		identities = append(identities, &v.SigningIdentity)
	}
	return identities
}

type VirtualHost struct {
	// Signing identity contains the server name, private key and key ID of
	// the virtual host.
	gomatrixserverlib.SigningIdentity `yaml:",inline"`

	// Path to the private key. If not specified, the default global private key
	// will be used instead.
	PrivateKeyPath Path `yaml:"private_key"`

	// How long a remote server can cache our server key for before requesting it again.
	// Increasing this number will reduce the number of requests made by remote servers
	// for our key, but increases the period a compromised key will be considered valid
	// by remote servers.
	// Defaults to 24 hours.
	KeyValidityPeriod time.Duration `yaml:"key_validity_period"`

	// Match these HTTP Host headers on the `/key/v2/server` endpoint, this needs
	// to match all delegated names, likely including the port number too if
	// the well-known delegation includes that also.
	MatchHTTPHosts []gomatrixserverlib.ServerName `yaml:"match_http_hosts"`

	// Is registration enabled on this virtual host?
	AllowRegistration bool `yaml:"allow_registration"`

	// Is guest registration enabled on this virtual host?
	AllowGuests bool `yaml:"allow_guests"`
}

func (v *VirtualHost) Verify(configErrs *ConfigErrors) {
	checkNotEmpty(configErrs, "virtual_host.*.server_name", string(v.ServerName))
}

// RegistrationAllowed returns two bools, the first states whether registration
// is allowed for this virtual host and the second states whether guests are
// allowed for this virtual host.
func (v *VirtualHost) RegistrationAllowed() (bool, bool) {
	if v == nil {
		return false, false
	}
	return v.AllowRegistration, v.AllowGuests
}

type OldVerifyKeys struct {
	// Path to the private key.
	PrivateKeyPath Path `yaml:"private_key"`

	// The private key itself.
	PrivateKey ed25519.PrivateKey `yaml:"-"`

	// The public key, in case only that part is known.
	PublicKey gomatrixserverlib.Base64Bytes `yaml:"public_key"`

	// The key ID of the private key.
	KeyID gomatrixserverlib.KeyID `yaml:"key_id"`

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

func (c *Metrics) Defaults(opts DefaultOpts) {
	c.Enabled = false
	if opts.Generate {
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
	AvatarURL string `yaml:"avatar_url"`
	// The roomname to be used when creating messages
	RoomName string `yaml:"room_name"`
}

func (c *ServerNotices) Defaults(opts DefaultOpts) {
	if opts.Generate {
		c.Enabled = true
		c.LocalPart = "_server"
		c.DisplayName = "Server Alert"
		c.RoomName = "Server Alert"
		c.AvatarURL = ""
	}
}

func (c *ServerNotices) Verify(errors *ConfigErrors, isMonolith bool) {}

type Cache struct {
	EstimatedMaxSize DataUnit      `yaml:"max_size_estimated"`
	MaxAge           time.Duration `yaml:"max_age"`
}

func (c *Cache) Defaults() {
	c.EstimatedMaxSize = 1024 * 1024 * 1024 // 1GB
	c.MaxAge = time.Hour
}

func (c *Cache) Verify(errors *ConfigErrors, isMonolith bool) {
	checkPositive(errors, "max_size_estimated", int64(c.EstimatedMaxSize))
}

// ReportStats configures opt-in phone-home statistics reporting.
type ReportStats struct {
	// Enabled configures phone-home statistics of the server
	Enabled bool `yaml:"enabled"`

	// Endpoint the endpoint to report stats to
	Endpoint string `yaml:"endpoint"`
}

func (c *ReportStats) Defaults() {
	c.Enabled = false
	c.Endpoint = "https://matrix.org/report-usage-stats/push"
}

func (c *ReportStats) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if c.Enabled {
		checkNotEmpty(configErrs, "global.report_stats.endpoint", c.Endpoint)
	}
}

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

// PresenceOptions defines possible configurations for presence events.
type PresenceOptions struct {
	// Whether inbound presence events are allowed
	EnableInbound bool `yaml:"enable_inbound"`
	// Whether outbound presence events are allowed
	EnableOutbound bool `yaml:"enable_outbound"`
}

type DataUnit int64

func (d *DataUnit) UnmarshalText(text []byte) error {
	var magnitude float64
	s := strings.ToLower(string(text))
	switch {
	case strings.HasSuffix(s, "tb"):
		s, magnitude = s[:len(s)-2], 1024*1024*1024*1024
	case strings.HasSuffix(s, "gb"):
		s, magnitude = s[:len(s)-2], 1024*1024*1024
	case strings.HasSuffix(s, "mb"):
		s, magnitude = s[:len(s)-2], 1024*1024
	case strings.HasSuffix(s, "kb"):
		s, magnitude = s[:len(s)-2], 1024
	default:
		magnitude = 1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*d = DataUnit(v * magnitude)
	return nil
}
