package test

import (
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/nats-io/nats.go"
)

func Base(cfg *config.Dendrite) (*base.BaseDendrite, nats.JetStreamContext, *nats.Conn) {
	if cfg == nil {
		cfg = &config.Dendrite{}
		cfg.Defaults(true)
	}
	cfg.Global.JetStream.InMemory = true
	base := base.NewBaseDendrite(cfg, "Tests")
	js, jc := base.NATS.Prepare(base.ProcessContext, &cfg.Global.JetStream)
	return base, js, jc
}
