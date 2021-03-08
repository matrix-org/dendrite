package config

import "fmt"

// Defined Kafka topics.
const (
	TopicOutputTypingEvent       = "OutputTypingEvent"
	TopicOutputSendToDeviceEvent = "OutputSendToDeviceEvent"
	TopicOutputKeyChangeEvent    = "OutputKeyChangeEvent"
	TopicOutputRoomEvent         = "OutputRoomEvent"
	TopicOutputClientData        = "OutputClientData"
	TopicOutputReceiptEvent      = "OutputReceiptEvent"
)

type Kafka struct {
	// A list of kafka addresses to connect to.
	Addresses []string `yaml:"addresses"`
	// The prefix to use for Kafka topic names for this homeserver - really only
	// useful if running more than one Dendrite on the same Kafka deployment.
	TopicPrefix string `yaml:"topic_prefix"`
	// Whether to use naffka instead of kafka.
	// Naffka can only be used when running dendrite as a single monolithic server.
	// Kafka can be used both with a monolithic server and when running the
	// components as separate servers.
	UseNaffka bool `yaml:"use_naffka"`
	// The Naffka database is used internally by the naffka library, if used.
	Database DatabaseOptions `yaml:"naffka_database"`
	// The max size a Kafka message passed between consumer/producer can have
	// Equals roughly max.message.bytes / fetch.message.max.bytes in Kafka
	MaxMessageBytes *int `yaml:"max_message_bytes"`
}

func (k *Kafka) TopicFor(name string) string {
	return fmt.Sprintf("%s%s", k.TopicPrefix, name)
}

func (c *Kafka) Defaults() {
	c.UseNaffka = true
	c.Database.Defaults(10)
	c.Addresses = []string{"localhost:2181"}
	c.Database.ConnectionString = DataSource("file:naffka.db")
	c.TopicPrefix = "Dendrite"

	maxBytes := 1024 * 1024 * 8 // about 8MB
	c.MaxMessageBytes = &maxBytes
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
	checkNotEmpty(configErrs, "global.kafka.topic_prefix", string(c.TopicPrefix))
	checkPositive(configErrs, "global.kafka.max_message_bytes", int64(*c.MaxMessageBytes))
}
