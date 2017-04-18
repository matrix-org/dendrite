package sync

import (
	"github.com/matrix-org/dendrite/syncserver/types"
	"net/http"
	"strconv"
	"time"
)

const defaultSyncTimeout = time.Duration(30) * time.Second
const defaultTimelineLimit = 20

// syncRequest represents a /sync request, with sensible defaults/sanity checks applied.
type syncRequest struct {
	userID        string
	limit         int
	timeout       time.Duration
	since         types.StreamPosition
	wantFullState bool
}

func newSyncRequest(req *http.Request, userID string) (*syncRequest, error) {
	timeout := getTimeout(req.URL.Query().Get("timeout"))
	fullState := req.URL.Query().Get("full_state")
	wantFullState := fullState != "" && fullState != "false"
	since, err := getSyncStreamPosition(req.URL.Query().Get("since"))
	if err != nil {
		return nil, err
	}
	// TODO: Additional query params: set_presence, filter
	return &syncRequest{
		userID:        userID,
		timeout:       timeout,
		since:         since,
		wantFullState: wantFullState,
		limit:         defaultTimelineLimit, // TODO: read from filter
	}, nil
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

func getSyncStreamPosition(since string) (types.StreamPosition, error) {
	if since == "" {
		return types.StreamPosition(0), nil
	}
	i, err := strconv.Atoi(since)
	if err != nil {
		return types.StreamPosition(0), err
	}
	return types.StreamPosition(i), nil
}
