package config

import (
	"fmt"
)

type JetStream struct {
	Matrix *Global `yaml:"-"`

	// Persistent directory to store JetStream streams in.
	StoragePath Path `yaml:"storage_path"`
	// A list of NATS addresses to connect to. If none are specified, an
	// internal NATS server will be used when running in monolith mode only.
	Addresses []string `yaml:"addresses"`
	// The prefix to use for stream names for this homeserver - really only
	// useful if running more than one Dendrite on the same NATS deployment.
	TopicPrefix string `yaml:"topic_prefix"`
	// Keep all storage in memory. This is mostly useful for unit tests.
	InMemory bool `yaml:"in_memory"`
	// Disable logging. This is mostly useful for unit tests.
	NoLog bool `yaml:"-"`
	// Disables TLS validation. This should NOT be used in production
	DisableTLSValidation bool `yaml:"disable_tls_validation"`
}

func (c *JetStream) Prefixed(name string) string {
	return fmt.Sprintf("%s%s", c.TopicPrefix, name)
}

func (c *JetStream) Durable(name string) string {
	return c.Prefixed(name)
}

func (c *JetStream) Defaults(opts DefaultOpts) {
	c.Addresses = []string{}
	c.TopicPrefix = "Dendrite"
	if opts.Generate {
		c.StoragePath = Path("./")
		c.NoLog = true
		c.DisableTLSValidation = true
	}
}

func (c *JetStream) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if isMonolith { // polylith required configs below
		return
	}
	// If we are running in a polylith deployment then we need at least
	// one NATS JetStream server to talk to.
	checkNotZero(configErrs, "global.jetstream.addresses", int64(len(c.Addresses)))
}
