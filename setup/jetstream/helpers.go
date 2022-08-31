package jetstream

import (
	"context"
	"fmt"

	"github.com/getsentry/sentry-go"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// JetStreamConsumer starts a durable consumer on the given subject with the
// given durable name. The function will be called when one or more messages
// is available, up to the maximum batch size specified. If the batch is set to
// 1 then messages will be delivered one at a time. If the function is called,
// the messages array is guaranteed to be at least 1 in size. Any provided NATS
// options will be passed through to the pull subscriber creation. The consumer
// will continue to run until the context expires, at which point it will stop.
func JetStreamConsumer(
	ctx context.Context, js nats.JetStreamContext, subj, durable string, batch int,
	f func(ctx context.Context, msgs []*nats.Msg) bool,
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

	// If the batch size is greater than 1, we will want to acknowledge all
	// received messages in the batch. Below we will send an acknowledgement
	// for the most recent message in the batch and AckAll will ensure that
	// all messages that came before it are also acknowledged implicitly.
	if batch > 1 {
		opts = append(opts, nats.AckAll())
	}

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
			msgs, err := sub.Fetch(batch, nats.Context(ctx))
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
			msg := msgs[len(msgs)-1] // most recent message, in case of AckAll
			if err = msg.InProgress(nats.Context(ctx)); err != nil {
				logrus.WithContext(ctx).WithField("subject", subj).Warn(fmt.Errorf("msg.InProgress: %w", err))
				sentry.CaptureException(err)
				continue
			}
			if f(ctx, msgs) {
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
