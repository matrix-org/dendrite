package jetstream

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
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
		sentry.CaptureException(err)
		return fmt.Errorf("nats.SubscribeSync: %w", err)
	}
	go func() {
		for {
			// If the parent context has given up then there's no point in
			// carrying on doing anything, so stop the listener.
			select {
			case <-ctx.Done():
				if err := sub.Unsubscribe(); err != nil {
					logrus.WithContext(ctx).Warnf("Failed to unsubscribe %q", durable)
				}
				return
			default:
			}
			// The context behaviour here is surprising — we supply a context
			// so that we can interrupt the fetch if we want, but NATS will still
			// enforce its own deadline (roughly 5 seconds by default). Therefore
			// it is our responsibility to check whether our context expired or
			// not when a context error is returned. Footguns. Footguns everywhere.
			msgs, err := sub.Fetch(1, nats.Context(ctx))
			if err != nil {
				if err == context.Canceled || err == context.DeadlineExceeded {
					// Work out whether it was the JetStream context that expired
					// or whether it was our supplied context.
					select {
					case <-ctx.Done():
						// The supplied context expired, so we want to stop the
						// consumer altogether.
						return
					default:
						// The JetStream context expired, so the fetch probably
						// just timed out and we should try again.
						continue
					}
				} else {
					// Something else went wrong, so we'll panic.
					sentry.CaptureException(err)
					logrus.WithContext(ctx).WithField("subject", subj).Fatal(err)
				}
			}
			if len(msgs) < 1 {
				continue
			}
			msg := msgs[0]
			if err = msg.InProgress(nats.Context(ctx)); err != nil {
				logrus.WithContext(ctx).WithField("subject", subj).Warn(fmt.Errorf("msg.InProgress: %w", err))
				sentry.CaptureException(err)
				continue
			}
			if f(ctx, msg) {
				if err = msg.AckSync(nats.Context(ctx)); err != nil {
					logrus.WithContext(ctx).WithField("subject", subj).Warn(fmt.Errorf("msg.AckSync: %w", err))
					sentry.CaptureException(err)
				}
			} else {
				if err = msg.Nak(nats.Context(ctx)); err != nil {
					logrus.WithContext(ctx).WithField("subject", subj).Warn(fmt.Errorf("msg.Nak: %w", err))
					sentry.CaptureException(err)
				}
			}
		}
	}()
	return nil
}
