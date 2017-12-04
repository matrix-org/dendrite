package othooks

import "context"
import "github.com/opentracing/opentracing-go"

import "github.com/opentracing/opentracing-go/ext"

type Hook struct {
	tracer opentracing.Tracer
}

func New(tracer opentracing.Tracer) *Hook {
	return &Hook{tracer: tracer}
}

func (h *Hook) Before(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
	parent := opentracing.SpanFromContext(ctx)
	if parent == nil {
		return ctx, nil
	}

	span := h.tracer.StartSpan("sql", opentracing.ChildOf(parent.Context()))
	ext.DBStatement.Set(span, query)

	return opentracing.ContextWithSpan(ctx, span), nil
}

func (h *Hook) After(ctx context.Context, query string, args ...interface{}) (context.Context, error) {
	span := opentracing.SpanFromContext(ctx)
	if span != nil {
		defer span.Finish()
	}

	return ctx, nil
}
