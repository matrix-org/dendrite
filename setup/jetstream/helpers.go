package jetstream

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

func JetStreamConsumer(
	ctx context.Context, nats nats.JetStreamContext, subj string,
	f func(ctx context.Context, msg *nats.Msg) bool,
	opts ...nats.SubOpt,
) error {
	sub, err := nats.SubscribeSync(subj, opts...)
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
			msg, err := sub.NextMsgWithContext(ctx)
			if err != nil {
				handle(fmt.Errorf("sub.NextMsgWithContext: %w", err))
			}
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
