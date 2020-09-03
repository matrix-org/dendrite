package routing

import (
	"net/http"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/util"
)

type rateLimits struct {
	limits           map[string]chan struct{}
	limitsMutex      sync.RWMutex
	enabled          bool
	requestThreshold int64
	cooloffDuration  time.Duration
}

func newRateLimits(cfg *config.RateLimiting) *rateLimits {
	l := &rateLimits{
		limits:           make(map[string]chan struct{}),
		enabled:          cfg.Enabled,
		requestThreshold: cfg.Threshold,
		cooloffDuration:  time.Duration(cfg.CooloffMS) * time.Millisecond,
	}
	if l.enabled {
		go l.clean()
	}
	return l
}

func (l *rateLimits) clean() {
	for {
		// On a 30 second interval, we'll take an exclusive write
		// lock of the entire map and see if any of the channels are
		// empty. If they are then we will close and delete them,
		// freeing up memory.
		time.Sleep(time.Second * 30)
		l.limitsMutex.Lock()
		for k, c := range l.limits {
			if len(c) == 0 {
				close(c)
				delete(l.limits, k)
			}
		}
		l.limitsMutex.Unlock()
	}
}

func (l *rateLimits) rateLimit(req *http.Request) *util.JSONResponse {
	// If rate limiting is disabled then do nothing.
	if !l.enabled {
		return nil
	}

	// Lock the map long enough to check for rate limiting. We hold it
	// for longer here than we really need to but it makes sure that we
	// also don't conflict with the cleaner goroutine which might clean
	// up a channel after we have retrieved it otherwise.
	l.limitsMutex.RLock()
	defer l.limitsMutex.RUnlock()

	// First of all, work out if X-Forwarded-For was sent to us. If not
	// then we'll just use the IP address of the caller.
	caller := req.RemoteAddr
	if forwardedFor := req.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		caller = forwardedFor
	}

	// Look up the caller's channel, if they have one. If they don't then
	// let's create one.
	rateLimit, ok := l.limits[caller]
	if !ok {
		l.limits[caller] = make(chan struct{}, l.requestThreshold)
		rateLimit = l.limits[caller]
	}

	// Check if the user has got free resource slots for this request.
	// If they don't then we'll return an error.
	select {
	case rateLimit <- struct{}{}:
	default:
		// We hit the rate limit. Tell the client to back off.
		return &util.JSONResponse{
			Code: http.StatusTooManyRequests,
			JSON: jsonerror.LimitExceeded("You are sending too many requests too quickly!", l.cooloffDuration.Milliseconds()),
		}
	}

	// After the time interval, drain a resource from the rate limiting
	// channel. This will free up space in the channel for new requests.
	go func() {
		<-time.After(l.cooloffDuration)
		<-rateLimit
	}()
	return nil
}
