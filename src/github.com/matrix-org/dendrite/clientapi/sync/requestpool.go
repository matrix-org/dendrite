package sync

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const defaultSyncTimeout = time.Duration(30) * time.Second

type syncRequest struct {
	userID        string
	timeout       time.Duration
	since         string
	wantFullState bool
}

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db *storage.SyncServerDatabase
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
	since := req.URL.Query().Get("since")
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

	res, err := rp.currentSyncForUser(syncReq)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

// OnNewEvent is called when a new event is received from the room server
func (rp *RequestPool) OnNewEvent(ev *gomatrixserverlib.Event, syncStreamPos int64) {
	fmt.Println("OnNewEvent =>", ev.EventID(), syncStreamPos)
}

func (rp *RequestPool) currentSyncForUser(req syncRequest) ([]gomatrixserverlib.Event, error) {
	// https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L179
	// Check if we are going to return immediately and if so, calculate the current
	// sync for this user and return.
	if req.since == "" || req.timeout == time.Duration(0) || req.wantFullState {
		return []gomatrixserverlib.Event{}, nil
	}

	// TODO: wait for an event which affects this user or one of their rooms, then recheck for new
	// sync data.
	time.Sleep(req.timeout)

	return nil, nil
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

// NewRequestPool makes a new RequestPool
func NewRequestPool(db *storage.SyncServerDatabase) RequestPool {
	return RequestPool{db}
}
