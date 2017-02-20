package util

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"runtime/debug"
	"time"

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

// JSONRequestHandler represents an interface that must be satisfied in order to respond to incoming
// HTTP requests with JSON. The interface returned will be marshalled into JSON to be sent to the client,
// unless the interface is []byte in which case the bytes are sent to the client unchanged.
// If an error is returned, a JSON error response will also be returned, unless the error code
// is a 302 REDIRECT in which case a redirect is sent based on the Message field.
type JSONRequestHandler interface {
	OnIncomingRequest(req *http.Request) (interface{}, *HTTPError)
}

// JSONError represents a JSON API error response
type JSONError struct {
	Message string `json:"message"`
}

// Protect panicking HTTP requests from taking down the entire process, and log them using
// the correct logger, returning a 500 with a JSON response rather than abruptly closing the
// connection. The http.Request MUST have a ctxValueLogger.
func Protect(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				logger := req.Context().Value(ctxValueLogger).(*log.Entry)
				logger.WithFields(log.Fields{
					"panic": r,
				}).Errorf(
					"Request panicked!\n%s", debug.Stack(),
				)
				jsonErrorResponse(
					w, req, &HTTPError{nil, "Internal Server Error", 500},
				)
			}
		}()
		handler(w, req)
	}
}

// MakeJSONAPI creates an HTTP handler which always responds to incoming requests with JSON responses.
// Incoming http.Requests will have a logger (with a request ID/method/path logged) attached to the Context.
// This can be accessed via GetLogger(Context). The type of the logger is *log.Entry from github.com/Sirupsen/logrus
func MakeJSONAPI(handler JSONRequestHandler) http.HandlerFunc {
	return Protect(func(w http.ResponseWriter, req *http.Request) {
		reqID := RandomString(12)
		// Set a Logger on the context
		ctx := context.WithValue(req.Context(), ctxValueLogger, log.WithFields(log.Fields{
			"req.method": req.Method,
			"req.path":   req.URL.Path,
			"req.id":     reqID,
		}))
		ctx = context.WithValue(ctx, ctxValueRequestID, reqID)
		req = req.WithContext(ctx)

		logger := req.Context().Value(ctxValueLogger).(*log.Entry)
		logger.Print("Incoming request")

		res, httpErr := handler.OnIncomingRequest(req)

		// Set common headers returned regardless of the outcome of the request
		w.Header().Set("Content-Type", "application/json")
		SetCORSHeaders(w)

		if httpErr != nil {
			jsonErrorResponse(w, req, httpErr)
			return
		}

		// if they've returned bytes as the response, then just return them rather than marshalling as JSON.
		// This gives handlers an escape hatch if they want to return cached bytes.
		var resBytes []byte
		resBytes, ok := res.([]byte)
		if !ok {
			r, err := json.Marshal(res)
			if err != nil {
				jsonErrorResponse(w, req, &HTTPError{nil, "Failed to serialise response as JSON", 500})
				return
			}
			resBytes = r
		}
		logger.Print(fmt.Sprintf("Responding (%d bytes)", len(resBytes)))
		w.Write(resBytes)
	})
}

func jsonErrorResponse(w http.ResponseWriter, req *http.Request, httpErr *HTTPError) {
	logger := req.Context().Value(ctxValueLogger).(*log.Entry)
	if httpErr.Code == 302 {
		logger.WithField("err", httpErr.Error()).Print("Redirecting")
		http.Redirect(w, req, httpErr.Message, 302)
		return
	}
	logger.WithFields(log.Fields{
		log.ErrorKey: httpErr,
	}).Print("Responding with error")

	w.WriteHeader(httpErr.Code) // Set response code

	r, err := json.Marshal(&JSONError{
		Message: httpErr.Message,
	})
	if err != nil {
		// We should never fail to marshal the JSON error response, but in this event just skip
		// marshalling altogether
		logger.Warn("Failed to marshal error response")
		w.Write([]byte(`{}`))
		return
	}
	w.Write(r)
}

// WithCORSOptions intercepts all OPTIONS requests and responds with CORS headers. The request handler
// is not invoked when this happens.
func WithCORSOptions(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "OPTIONS" {
			SetCORSHeaders(w)
			return
		}
		handler(w, req)
	}
}

// SetCORSHeaders sets unrestricted origin Access-Control headers on the response writer
func SetCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
}

const alphanumerics = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomString generates a pseudo-random string of length n.
func RandomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphanumerics[rand.Int63()%int64(len(alphanumerics))]
	}
	return string(b)
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}
