package sync

import (
	"context"
	"fmt"
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
			JSON: jsonerror.InvalidSync(err.Error()),
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
		"current": rp.currPos,
	}).Info("Incoming /sync request")

	// set the timeout going
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()

	res, err := rp.currentSyncForUser(ctx, syncReq)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

// OnNewEvent is called when a new event is received from the room server. Must only be
// called from a single goroutine.
func (rp *RequestPool) OnNewEvent(ev *gomatrixserverlib.Event, pos syncStreamPosition) {
	// update the current position in a guard and then notify all /sync streams
	rp.cond.L.Lock()
	fmt.Println("OnNewEvent =>", ev.EventID(), " pos=", pos, " old_pos=", rp.currPos)
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

func (rp *RequestPool) currentSyncForUser(ctx context.Context, req syncRequest) ([]gomatrixserverlib.Event, error) {
	currentPos := rp.waitForEvents(req)
	return rp.db.EventsInRange(int64(req.since), int64(currentPos))

	// https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L179
	// Check if we are going to return immediately and if so, calculate the current
	// sync for this user and return.
	// if req.since == 0 || req.timeout == time.Duration(0) || req.wantFullState {
	// 	return []gomatrixserverlib.Event{}, nil
	// }

	// Steps: (no token)
	// - get all rooms the user is joined to.
	// - f.e room, get room state.
	// - f.e room, get latest N messages.
	// - rollback state by N messages.

	// Steps: (up-to-date token)
	// - Wait for new event.
	// - Check if event should notify user.
	// - Notify user or continue waiting, eventually timing out.

	// Steps: (partial token, less than threshold)
	// - Get rooms the user is joined to.
	// - Get all events between token and now for those rooms.
	// - Work out state and message delta and return.

	// Steps: (partial token, more than threshold (too expensive to do the above))
	// - Ignore for now, meaning this code path will be horrendously slow.
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
