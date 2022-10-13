package process

import (
	"context"
	"sync"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
)

type ProcessContext struct {
	mu       sync.RWMutex
	wg       *sync.WaitGroup     // used to wait for components to shutdown
	ctx      context.Context     // cancelled when Stop is called
	shutdown context.CancelFunc  // shut down Dendrite
	degraded map[string]struct{} // reasons why the process is degraded
}

func NewProcessContext() *ProcessContext {
	ctx, shutdown := context.WithCancel(context.Background())
	return &ProcessContext{
		ctx:      ctx,
		shutdown: shutdown,
		wg:       &sync.WaitGroup{},
	}
}

func (b *ProcessContext) Context() context.Context {
	return context.WithValue(b.ctx, "scope", "process") // nolint:staticcheck
}

func (b *ProcessContext) ComponentStarted() {
	b.wg.Add(1)
}

func (b *ProcessContext) ComponentFinished() {
	b.wg.Done()
}

func (b *ProcessContext) ShutdownDendrite() {
	b.shutdown()
}

func (b *ProcessContext) WaitForShutdown() <-chan struct{} {
	return b.ctx.Done()
}

func (b *ProcessContext) WaitForComponentsToFinish() {
	b.wg.Wait()
}

func (b *ProcessContext) Degraded(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.degraded[err.Error()]; !ok {
		logrus.WithError(err).Warn("Dendrite has entered a degraded state")
		sentry.CaptureException(err)
		b.degraded[err.Error()] = struct{}{}
	}
}

func (b *ProcessContext) IsDegraded() (bool, []string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.degraded) == 0 {
		return false, nil
	}
	reasons := make([]string, 0, len(b.degraded))
	for reason := range b.degraded {
		reasons = append(reasons, reason)
	}
	return true, reasons
}
