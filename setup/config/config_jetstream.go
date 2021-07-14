package config

import "fmt"

type JetStream struct {
	Matrix *Global `yaml:"-"`

	// A list of NATS addresses to connect to. If none are specified, an
	// internal NATS server will be used when running in monolith mode only.
	Addresses []string `yaml:"addresses"`
	// The prefix to use for stream names for this homeserver - really only
	// useful if running more than one Dendrite on the same NATS deployment.
	TopicPrefix string `yaml:"topic_prefix"`
}

func (k *JetStream) TopicFor(name string) string {
	return fmt.Sprintf("%s%s", k.TopicPrefix, name)
}

func (c *JetStream) Defaults() {
	c.Addresses = []string{}
	c.TopicPrefix = "Dendrite"
}

func (c *JetStream) Verify(configErrs *ConfigErrors, isMonolith bool) {
	// If we are running in a polylith deployment then we need at least
	// one NATS JetStream server to talk to.
	if !isMonolith {
		checkNotZero(configErrs, "global.jetstream.addresses", int64(len(c.Addresses)))
	}
}
