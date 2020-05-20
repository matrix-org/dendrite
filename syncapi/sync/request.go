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
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

const defaultSyncTimeout = time.Duration(0)
const defaultTimelineLimit = 20

type filter struct {
	Room struct {
		Timeline struct {
			Limit *int `json:"limit"`
		} `json:"timeline"`
	} `json:"room"`
}

// syncRequest represents a /sync request, with sensible defaults/sanity checks applied.
type syncRequest struct {
	ctx           context.Context
	device        authtypes.Device
	limit         int
	timeout       time.Duration
	since         *types.StreamingToken // nil means that no since token was supplied
	wantFullState bool
	log           *log.Entry
}

func newSyncRequest(req *http.Request, device authtypes.Device) (*syncRequest, error) {
	timeout := getTimeout(req.URL.Query().Get("timeout"))
	fullState := req.URL.Query().Get("full_state")
	wantFullState := fullState != "" && fullState != "false"
	var since *types.StreamingToken
	sinceStr := req.URL.Query().Get("since")
	if sinceStr != "" {
		tok, err := types.NewStreamTokenFromString(sinceStr)
		if err != nil {
			return nil, err
		}
		since = &tok
	}
	timelineLimit := defaultTimelineLimit
	// TODO: read from stored filters too
	filterQuery := req.URL.Query().Get("filter")
	if filterQuery != "" && filterQuery[0] == '{' {
		// attempt to parse the timeline limit at least
		var f filter
		err := json.Unmarshal([]byte(filterQuery), &f)
		if err == nil && f.Room.Timeline.Limit != nil {
			timelineLimit = *f.Room.Timeline.Limit
		}
	}
	// TODO: Additional query params: set_presence, filter
	return &syncRequest{
		ctx:           req.Context(),
		device:        device,
		timeout:       timeout,
		since:         since,
		wantFullState: wantFullState,
		limit:         timelineLimit,
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
