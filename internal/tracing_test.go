package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTracing(t *testing.T) {
	inCtx := context.Background()

	task, ctx := StartTask(inCtx, "testing")
	assert.NotNil(t, ctx)
	assert.NotNil(t, task)
	assert.NotEqual(t, inCtx, ctx)
	task.SetTag("key", "value")

	region, ctx2 := StartRegion(ctx, "testing")
	assert.NotNil(t, ctx)
	assert.NotNil(t, region)
	assert.NotEqual(t, ctx, ctx2)
	defer task.EndTask()
	defer region.EndRegion()
}
