package jetstream

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

func JetStreamConsumer(
	ctx context.Context, js nats.JetStreamContext, subj, durable string,
	f func(ctx context.Context, msg *nats.Msg) bool,
	opts ...nats.SubOpt,
) error {
	defer func() {
		// If there are existing consumers from before they were pull
		// consumers, we need to clean up the old push consumers. However,
		// in order to not affect the interest-based policies, we need to
		// do this *after* creating the new pull consumers, which have
		// "Pull" suffixed to their name.
		if _, err := js.ConsumerInfo(subj, durable); err == nil {
			if err := js.DeleteConsumer(subj, durable); err != nil {
				logrus.WithContext(ctx).Warnf("Failed to clean up old consumer %q", durable)
			}
		}
	}()

	name := durable + "Pull"
	sub, err := js.PullSubscribe(subj, name, opts...)
	if err != nil {
		return fmt.Errorf("nats.SubscribeSync: %w", err)
	}
	go func() {
		handle := func(err error) {
			if err == context.Canceled || err == context.DeadlineExceeded {
				logrus.WithContext(ctx).WithField("subject", subj).Warn(err)
				return
			}
			logrus.WithContext(ctx).WithField("subject", subj).Fatal(err)
		}
		for {
			msgs, err := sub.Fetch(1, nats.Context(ctx))
			if err != nil {
				handle(fmt.Errorf("sub.Fetch: %w", err))
			}
			if len(msgs) < 1 {
				continue
			}
			msg := msgs[0]
			if err = msg.InProgress(); err != nil {
				handle(fmt.Errorf("msg.InProgress: %w", err))
			}
			if f(ctx, msg) {
				if err = msg.Ack(); err != nil {
					handle(fmt.Errorf("msg.Ack: %w", err))
				}
			} else {
				if err = msg.Nak(); err != nil {
					handle(fmt.Errorf("msg.Nak: %w", err))
				}
			}
		}
	}()
	return nil
}
