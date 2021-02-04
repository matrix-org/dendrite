package process

import (
	"context"
	"sync"
)

type ProcessContext struct {
	wg       *sync.WaitGroup    // used to wait for components to shutdown
	ctx      context.Context    // cancelled when Stop is called
	shutdown context.CancelFunc // shut down Dendrite
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
