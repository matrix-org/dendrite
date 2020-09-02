package routing

import (
	"net/http"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

var clientRateLimits sync.Map // device ID -> chan bool buffered
var clientRateLimitMaxRequests = 10
var clientRateLimitTimeIntervalMS = time.Millisecond * 500

func rateLimit(req *http.Request) *util.JSONResponse {
	// Check if the user has got free resource slots for this request.
	// If they don't then we'll return an error.
	rateLimit, _ := clientRateLimits.LoadOrStore(req.RemoteAddr, make(chan struct{}, clientRateLimitMaxRequests))
	rateChan := rateLimit.(chan struct{})
	select {
	case rateChan <- struct{}{}:
	default:
		// We hit the rate limit. Tell the client to back off.
		return &util.JSONResponse{
			Code: http.StatusTooManyRequests,
			JSON: jsonerror.LimitExceeded("You are sending too many requests too quickly!", clientRateLimitTimeIntervalMS.Milliseconds()),
		}
	}

	// After the time interval, drain a resource from the rate limiting
	// channel. This will free up space in the channel for new requests.
	go func() {
		<-time.After(clientRateLimitTimeIntervalMS)
		<-rateChan

		// TODO: racy?
		if len(rateChan) == 0 {
			close(rateChan)
			clientRateLimits.Delete(req.RemoteAddr)
		}
	}()
	return nil
}
