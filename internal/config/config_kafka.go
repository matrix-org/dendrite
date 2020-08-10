package config

import "fmt"

// Defined Kafka topics.
const (
	TopicOutputTypingEvent       = "OutputTypingEvent"
	TopicOutputSendToDeviceEvent = "OutputSendToDeviceEvent"
	TopicOutputKeyChangeEvent    = "OutputKeyChangeEvent"
	TopicOutputRoomEvent         = "OutputRoomEvent"
	TopicOutputClientData        = "OutputClientData"
)

type Kafka struct {
	Addresses   []string        `json:"Addresses" comment:"List of Kafka addresses to connect to."`
	TopicPrefix string          `json:"topic_prefix" comment:"The prefix to use for Kafka topic names for this homeserver. Change this only if\nyou are running more than one Dendrite homeserver on the same Kafka deployment."`
	UseNaffka   bool            `json:"UseNaffka" comment:"Whether to use Naffka instead of Kafka. Only available in monolith mode."`
	Database    DatabaseOptions `json:"NaffkaDatabase" comment:"Naffka database options. Not required when using Kafka."`
}

func (k *Kafka) TopicFor(name string) string {
	return fmt.Sprintf("%s%s", k.TopicPrefix, name)
}

func (c *Kafka) Defaults() {
	c.UseNaffka = true
	c.Database.Defaults()
	c.Database.ConnectionString = DataSource("file:naffka.db")
	c.TopicPrefix = "Dendrite"
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
}
