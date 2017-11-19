// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sync

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

const defaultSyncTimeout = time.Duration(30) * time.Second
const defaultTimelineLimit = 20

// syncRequest represents a /sync request, with sensible defaults/sanity checks applied.
type syncRequest struct {
	ctx           context.Context
	userID        string
	limit         int
	timeout       time.Duration
	since         types.StreamPosition
	wantFullState bool
	log           *log.Entry
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
		ctx:           req.Context(),
		userID:        userID,
		timeout:       timeout,
		since:         since,
		wantFullState: wantFullState,
		limit:         defaultTimelineLimit, // TODO: read from filter
		log:           util.GetLogger(req.Context()),
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
