package util

import (
	"context"

	log "github.com/Sirupsen/logrus"
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

// GetLogger retrieves the logrus logger from the supplied context. Returns nil if there is no logger.
func GetLogger(ctx context.Context) *log.Entry {
	l := ctx.Value(ctxValueLogger)
	if l == nil {
		return nil
	}
	return l.(*log.Entry)
}
