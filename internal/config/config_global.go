package config

import (
	"math/rand"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"golang.org/x/crypto/ed25519"
)

type Global struct {
	ServerName        gomatrixserverlib.ServerName `json:"ServerName" comment:"The domain name of this homeserver."`
	PrivateKeyPath    Path                         `json:"PrivateKeyPath" comment:"The path to the signing private key file, used to sign requests and events."`
	PrivateKey        ed25519.PrivateKey           `json:"-"`
	KeyID             gomatrixserverlib.KeyID      `json:"KeyID" comment:"A unique identifier for this private key. Must start with the prefix \"ed25519:\"."`
	KeyValidityPeriod time.Duration                `json:"KeyValidityPeriod" comment:"How long a remote server can cache our server signing key before requesting it\nagain. Increasing this number will reduce the number of requests made by other\nservers for our key but increases the period that a compromised key will be\nconsidered valid by other homeservers."`
	TrustedIDServers  []string                     `json:"TrustedIDServers" comment:"Lists of domains that the server will trust as identity servers to verify third\nparty identifiers such as phone numbers and email addresses."`
	Kafka             Kafka                        `json:"Kafka" comment:"Configuration for Kaffka/Naffka."`
	Metrics           Metrics                      `json:"Metrics" comment:"Configuration for Prometheus metric collection."`
}

func (c *Global) Defaults() {
	c.ServerName = "localhost"
	c.PrivateKeyPath = "matrix.pem"
	_, c.PrivateKey, _ = ed25519.GenerateKey(rand.New(rand.NewSource(0)))
	c.KeyID = "ed25519:auto"
	c.KeyValidityPeriod = time.Hour * 24 * 7

	c.Kafka.Defaults()
	c.Metrics.Defaults()
}

func (c *Global) Verify(configErrs *ConfigErrors, isMonolith bool) {
	checkNotEmpty(configErrs, "Global.ServerName", string(c.ServerName))
	checkNotEmpty(configErrs, "Global.PrivateKeyPath", string(c.PrivateKeyPath))

	c.Kafka.Verify(configErrs, isMonolith)
	c.Metrics.Verify(configErrs, isMonolith)
}

type BasicAuth struct {
	Username string `json:"Username"`
	Password string `json:"Password"`
}

// The configuration to use for Prometheus metrics
type Metrics struct {
	Enabled   bool      `json:"Enabled" comment:"Whether or not Prometheus metrics are enabled."`
	BasicAuth BasicAuth `json:"BasicAuth" comment:"HTTP basic authentication to protect access to monitoring."`
}

func (c *Metrics) Defaults() {
	c.Enabled = false
	c.BasicAuth.Username = "metrics"
	c.BasicAuth.Password = "metrics"
}

func (c *Metrics) Verify(configErrs *ConfigErrors, isMonolith bool) {
}

type DatabaseOptions struct {
	ConnectionString       DataSource `json:"ConnectionString" comment:"Connection string, e.g. file:filename.db or postgresql://user:pass@host/db etc."`
	MaxOpenConnections     int        `json:"MaxOpenConnections" comment:"Maximum number of connections to open to the database (0 = use default,\nnegative = unlimited)."`
	MaxIdleConnections     int        `json:"MaxIdleConnections" comment:"Maximum number of idle connections permitted to the database (0 = use default,\nnegative = unlimited)."`
	ConnMaxLifetimeSeconds int        `json:"ConnMaxLifetimeSeconds" comment:"Maximum amount of time, in seconds, that a database connection may be reused\n(negative = unlimited)."`
}

func (c *DatabaseOptions) Defaults() {
	c.MaxOpenConnections = 100
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
