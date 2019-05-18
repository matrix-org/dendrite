package util

import (
	"context"

	log "github.com/sirupsen/logrus"
)

// contextKeys is a type alias for string to namespace Context keys per-package.
type contextKeys string

// ctxValueRequestID is the key to extract the request ID for an HTTP request
const ctxValueRequestID = contextKeys("requestid")

// GetRequestID returns the request ID associated with this context, or the empty string
// if one is not associated with this context.
func GetRequestID(ctx context.Context) string {
	id := ctx.Value(ctxValueRequestID)
	if id == nil {
		return ""
	}
	return id.(string)
}

// ctxValueLogger is the key to extract the logrus Logger.
const ctxValueLogger = contextKeys("logger")

// GetLogger retrieves the logrus logger from the supplied context. Always returns a logger,
// even if there wasn't one originally supplied.
func GetLogger(ctx context.Context) *log.Entry {
	l := ctx.Value(ctxValueLogger)
	if l == nil {
		// Always return a logger so callers don't need to constantly nil check.
		return log.WithField("context", "missing")
	}
	return l.(*log.Entry)
}

// ContextWithLogger creates a new context, which will use the given logger.
func ContextWithLogger(ctx context.Context, l *log.Entry) context.Context {
	return context.WithValue(ctx, ctxValueLogger, l)
}
