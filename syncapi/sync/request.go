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

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

const defaultSyncTimeout = time.Duration(0)
const DefaultTimelineLimit = 20

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
	device        userapi.Device
	limit         int
	timeout       time.Duration
	since         *types.StreamingToken // nil means that no since token was supplied
	wantFullState bool
	log           *log.Entry
}

func newSyncRequest(req *http.Request, device userapi.Device, syncDB storage.Database) (*syncRequest, error) {
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
	if since == nil {
		tok := types.NewStreamToken(0, 0, nil)
		since = &tok
	}
	timelineLimit := DefaultTimelineLimit
	// TODO: read from stored filters too
	filterQuery := req.URL.Query().Get("filter")
	if filterQuery != "" {
		if filterQuery[0] == '{' {
			// attempt to parse the timeline limit at least
			var f filter
			err := json.Unmarshal([]byte(filterQuery), &f)
			if err == nil && f.Room.Timeline.Limit != nil {
				timelineLimit = *f.Room.Timeline.Limit
			}
		} else {
			// attempt to load the filter ID
			localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
			if err != nil {
				util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
				return nil, err
			}
			f, err := syncDB.GetFilter(req.Context(), localpart, filterQuery)
			if err == nil {
				timelineLimit = f.Room.Timeline.Limit
			}
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
