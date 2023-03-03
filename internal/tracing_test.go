package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTracing(t *testing.T) {
	inCtx := context.Background()

	tr, ctx := StartRegion(inCtx, "testing")
	assert.NotNil(t, ctx)
	assert.NotEqual(t, inCtx, ctx)
	assert.NotNil(t, tr)
	tr.SetTag("key", "value")
	defer tr.End()
}
