package config

import (
	"math/rand"
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

	// How long a remote server can cache our server key for before requesting it again.
	// Increasing this number will reduce the number of requests made by remote servers
	// for our key, but increases the period a compromised key will be considered valid
	// by remote servers.
	// Defaults to 24 hours.
	KeyValidityPeriod time.Duration `yaml:"key_validity_period"`

	// List of domains that the server will trust as identity servers to
	// verify third-party identifiers.
	// Defaults to an empty array.
	TrustedIDServers []string `yaml:"trusted_third_party_id_servers"`

	// Kafka/Naffka configuration
	Kafka Kafka `yaml:"kafka"`

	// Metrics configuration
	Metrics Metrics `yaml:"metrics"`
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
	checkNotEmpty(configErrs, "global.server_name", string(c.ServerName))
	checkNotEmpty(configErrs, "global.private_key", string(c.PrivateKeyPath))

	c.Kafka.Verify(configErrs, isMonolith)
	c.Metrics.Verify(configErrs, isMonolith)
}

type Kafka struct {
	// A list of kafka addresses to connect to.
	Addresses []string `yaml:"addresses"`
	// Whether to use naffka instead of kafka.
	// Naffka can only be used when running dendrite as a single monolithic server.
	// Kafka can be used both with a monolithic server and when running the
	// components as separate servers.
	UseNaffka bool `yaml:"use_naffka"`
	// The Naffka database is used internally by the naffka library, if used.
	Database DatabaseOptions `yaml:"naffka_database"`
	// The names of the topics to use when reading and writing from kafka.
	Topics struct {
		// Topic for roomserver/api.OutputRoomEvent events.
		OutputRoomEvent Topic `yaml:"output_room_event"`
		// Topic for sending account data from client API to sync API
		OutputClientData Topic `yaml:"output_client_data"`
		// Topic for eduserver/api.OutputTypingEvent events.
		OutputTypingEvent Topic `yaml:"output_typing_event"`
		// Topic for eduserver/api.OutputSendToDeviceEvent events.
		OutputSendToDeviceEvent Topic `yaml:"output_send_to_device_event"`
		// Topic for keyserver when new device keys are added.
		OutputKeyChangeEvent Topic `yaml:"output_key_change_event"`
	}
}

func (c *Kafka) Defaults() {
	c.UseNaffka = true
	c.Database.Defaults()
	c.Database.ConnectionString = DataSource("file:naffka.db")
	c.Topics.OutputRoomEvent = "OutputRoomEventTopic"
	c.Topics.OutputClientData = "OutputClientDataTopic"
	c.Topics.OutputTypingEvent = "OutputTypingEventTopic"
	c.Topics.OutputSendToDeviceEvent = "OutputSendToDeviceEventTopic"
	c.Topics.OutputKeyChangeEvent = "OutputKeyChangeEventTopic"
}

func (c *Kafka) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if c.UseNaffka {
		if !isMonolith {
			configErrs.Add("naffka can only be used in a monolithic server")
		}
		checkNotEmpty(configErrs, "global.kafka.database.connection_string", string(c.Database.ConnectionString))
	} else {
		// If we aren't using naffka then we need to have at least one kafka
		// server to talk to.
		checkNotZero(configErrs, "global.kafka.addresses", int64(len(c.Addresses)))
	}
	checkNotEmpty(configErrs, "global.kafka.topics.output_room_event", string(c.Topics.OutputRoomEvent))
	checkNotEmpty(configErrs, "global.kafka.topics.output_client_data", string(c.Topics.OutputClientData))
	checkNotEmpty(configErrs, "global.kafka.topics.output_typing_event", string(c.Topics.OutputTypingEvent))
	checkNotEmpty(configErrs, "global.kafka.topics.output_send_to_device_event", string(c.Topics.OutputSendToDeviceEvent))
	checkNotEmpty(configErrs, "global.kafka.topics.output_key_change_event", string(c.Topics.OutputKeyChangeEvent))
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

func (c *Metrics) Defaults() {
	c.Enabled = false
	c.BasicAuth.Username = "metrics"
	c.BasicAuth.Password = "metrics"
}

func (c *Metrics) Verify(configErrs *ConfigErrors, isMonolith bool) {
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
