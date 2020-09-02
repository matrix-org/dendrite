package routing

import (
	"net/http"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

type rateLimits struct {
	limits       map[string]chan struct{}
	limitsMutex  sync.RWMutex
	maxRequests  int
	timeInterval time.Duration
}

func newRateLimits() *rateLimits {
	l := &rateLimits{
		limits:       make(map[string]chan struct{}),
		maxRequests:  10,
		timeInterval: time.Millisecond * 500,
	}
	go l.clean()
	return l
}

func (l *rateLimits) clean() {
	for {
		time.Sleep(time.Second * 10)
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
	// Check if the user has got free resource slots for this request.
	// If they don't then we'll return an error.
	l.limitsMutex.RLock()
	defer l.limitsMutex.RUnlock()

	rateLimit, ok := l.limits[req.RemoteAddr]
	if !ok {
		l.limits[req.RemoteAddr] = make(chan struct{}, l.maxRequests)
		rateLimit = l.limits[req.RemoteAddr]
	}
	select {
	case rateLimit <- struct{}{}:
	default:
		// We hit the rate limit. Tell the client to back off.
		return &util.JSONResponse{
			Code: http.StatusTooManyRequests,
			JSON: jsonerror.LimitExceeded("You are sending too many requests too quickly!", l.timeInterval.Milliseconds()),
		}
	}

	// After the time interval, drain a resource from the rate limiting
	// channel. This will free up space in the channel for new requests.
	go func() {
		<-time.After(l.timeInterval)
		<-rateLimit
	}()
	return nil
}
