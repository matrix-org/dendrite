package sync

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const defaultSyncTimeout = time.Duration(30) * time.Second

type syncRequest struct {
	userID        string
	timeout       time.Duration
	since         syncStreamPosition
	wantFullState bool
}

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db *storage.SyncServerDatabase
	// The latest sync stream position: guarded by 'cond'.
	currPos syncStreamPosition
	// A condition variable to notify all waiting goroutines of a new sync stream position
	cond *sync.Cond
}

// NewRequestPool makes a new RequestPool
func NewRequestPool(db *storage.SyncServerDatabase) (*RequestPool, error) {
	pos, err := db.SyncStreamPosition()
	if err != nil {
		return nil, err
	}
	return &RequestPool{db, syncStreamPosition(pos), sync.NewCond(&sync.Mutex{})}, nil
}

// OnIncomingSyncRequest is called when a client makes a /sync request. This function MUST be
// called in a dedicated goroutine for this request. This function will block the goroutine
// until a response is ready, or it times out.
func (rp *RequestPool) OnIncomingSyncRequest(req *http.Request) util.JSONResponse {
	// Extract values from request
	logger := util.GetLogger(req.Context())
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return *resErr
	}
	since, err := getSyncStreamPosition(req.URL.Query().Get("since"))
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}
	timeout := getTimeout(req.URL.Query().Get("timeout"))
	fullState := req.URL.Query().Get("full_state")
	wantFullState := fullState != "" && fullState != "false"
	// TODO: Additional query params: set_presence, filter
	syncReq := syncRequest{
		userID:        userID,
		timeout:       timeout,
		since:         since,
		wantFullState: wantFullState,
	}
	logger.WithFields(log.Fields{
		"userID":  userID,
		"since":   since,
		"timeout": timeout,
	}).Info("Incoming /sync request")

	// Fork off 2 goroutines: one to do the work, and one to serve as a timeout.
	// Whichever returns first is the one we will serve back to the client.
	// TODO: Currently this means that cpu work is timed, which may not be what we want long term.
	timeoutChan := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		close(timeoutChan) // signal that the timeout has expired
	})

	done := make(chan util.JSONResponse)
	go func() {
		syncData, err := rp.currentSyncForUser(syncReq)
		timer.Stop()
		var res util.JSONResponse
		if err != nil {
			res = httputil.LogThenError(req, err)
		} else {
			res = util.JSONResponse{
				Code: 200,
				JSON: syncData,
			}
		}
		done <- res
		close(done)
	}()

	select {
	case <-timeoutChan: // timeout fired
		return util.JSONResponse{
			Code: 200,
			JSON: []struct{}{}, // return empty array for now
		}
	case res := <-done: // received a response
		return res
	}
}

// OnNewEvent is called when a new event is received from the room server. Must only be
// called from a single goroutine, to avoid races between updates which could set the
// current position in the stream incorrectly.
func (rp *RequestPool) OnNewEvent(ev *gomatrixserverlib.Event, pos syncStreamPosition) {
	// update the current position in a guard and then notify all /sync streams
	rp.cond.L.Lock()
	rp.currPos = pos
	rp.cond.L.Unlock()

	rp.cond.Broadcast() // notify ALL waiting goroutines
}

func (rp *RequestPool) waitForEvents(req syncRequest) syncStreamPosition {
	// In a guard, check if the /sync request should block, and block it until we get a new position
	rp.cond.L.Lock()
	currentPos := rp.currPos
	for req.since == currentPos {
		// we need to wait for a new event.
		// TODO: This waits for ANY new event, we need to only wait for events which we care about.
		rp.cond.Wait() // atomically unlocks and blocks goroutine, then re-acquires lock on unblock
		currentPos = rp.currPos
	}
	rp.cond.L.Unlock()
	return currentPos
}

func (rp *RequestPool) currentSyncForUser(req syncRequest) ([]gomatrixserverlib.Event, error) {
	currentPos := rp.waitForEvents(req)
	return rp.db.EventsInRange(int64(req.since), int64(currentPos))
}

func getTimeout(timeoutMS string) time.Duration {
	if timeoutMS == "" {
		return defaultSyncTimeout
	}
	i, err := strconv.Atoi(timeoutMS)
	if err != nil {
		return defaultSyncTimeout
	}
	return time.Duration(i) * time.Millisecond
}

func getSyncStreamPosition(since string) (syncStreamPosition, error) {
	if since == "" {
		return syncStreamPosition(0), nil
	}
	i, err := strconv.Atoi(since)
	if err != nil {
		return syncStreamPosition(0), err
	}
	return syncStreamPosition(i), nil
}
