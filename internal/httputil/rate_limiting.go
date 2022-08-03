package httputil

import (
	"net/http"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

type RateLimits struct {
	limits           map[string]chan struct{}
	limitsMutex      sync.RWMutex
	cleanMutex       sync.RWMutex
	enabled          bool
	requestThreshold int64
	cooloffDuration  time.Duration
	exemptUserIDs    map[string]struct{}
}

func NewRateLimits(cfg *config.RateLimiting) *RateLimits {
	l := &RateLimits{
		limits:           make(map[string]chan struct{}),
		enabled:          cfg.Enabled,
		requestThreshold: cfg.Threshold,
		cooloffDuration:  time.Duration(cfg.CooloffMS) * time.Millisecond,
		exemptUserIDs:    map[string]struct{}{},
	}
	for _, userID := range cfg.ExemptUserIDs {
		l.exemptUserIDs[userID] = struct{}{}
	}
	if l.enabled {
		go l.clean()
	}
	return l
}

func (l *RateLimits) clean() {
	for {
		// On a 30 second interval, we'll take an exclusive write
		// lock of the entire map and see if any of the channels are
		// empty. If they are then we will close and delete them,
		// freeing up memory.
		time.Sleep(time.Second * 30)
		l.cleanMutex.Lock()
		l.limitsMutex.Lock()
		for k, c := range l.limits {
			if len(c) == 0 {
				close(c)
				delete(l.limits, k)
			}
		}
		l.limitsMutex.Unlock()
		l.cleanMutex.Unlock()
	}
}

func (l *RateLimits) Limit(req *http.Request, device *userapi.Device) *util.JSONResponse {
	// If rate limiting is disabled then do nothing.
	if !l.enabled {
		return nil
	}

	// Take a read lock out on the cleaner mutex. The cleaner expects to
	// be able to take a write lock, which isn't possible while there are
	// readers, so this has the effect of blocking the cleaner goroutine
	// from doing its work until there are no requests in flight.
	l.cleanMutex.RLock()
	defer l.cleanMutex.RUnlock()

	// First of all, work out if X-Forwarded-For was sent to us. If not
	// then we'll just use the IP address of the caller.
	var caller string
	if device != nil {
		switch device.AccountType {
		case userapi.AccountTypeAdmin:
			return nil // don't rate-limit server administrators
		case userapi.AccountTypeAppService:
			return nil // don't rate-limit appservice users
		default:
			if _, ok := l.exemptUserIDs[device.UserID]; ok {
				// If the user is exempt from rate limiting then do nothing.
				return nil
			}
			caller = device.UserID + device.ID
		}
	} else {
		if forwardedFor := req.Header.Get("X-Forwarded-For"); forwardedFor != "" {
			caller = forwardedFor
		} else {
			caller = req.RemoteAddr
		}
	}

	// Look up the caller's channel, if they have one.
	l.limitsMutex.RLock()
	rateLimit, ok := l.limits[caller]
	l.limitsMutex.RUnlock()

	// If the caller doesn't have a channel, create one and write it
	// back to the map.
	if !ok {
		rateLimit = make(chan struct{}, l.requestThreshold)

		l.limitsMutex.Lock()
		l.limits[caller] = rateLimit
		l.limitsMutex.Unlock()
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
