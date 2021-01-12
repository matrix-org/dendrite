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
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
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

func newSyncRequest(req *http.Request, device userapi.Device, syncDB storage.Database) (*types.SyncRequest, error) {
	timeout := getTimeout(req.URL.Query().Get("timeout"))
	fullState := req.URL.Query().Get("full_state")
	wantFullState := fullState != "" && fullState != "false"
	since, sinceStr := types.StreamingToken{}, req.URL.Query().Get("since")
	if sinceStr != "" {
		var err error
		since, err = types.NewStreamTokenFromString(sinceStr)
		if err != nil {
			return nil, err
		}
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

	filter := gomatrixserverlib.DefaultEventFilter()
	filter.Limit = timelineLimit
	// TODO: Additional query params: set_presence, filter

	logger := util.GetLogger(req.Context()).WithFields(logrus.Fields{
		"user_id":   device.UserID,
		"device_id": device.ID,
		"since":     since,
		"timeout":   timeout,
		"limit":     timelineLimit,
	})

	return &types.SyncRequest{
		Context:       req.Context(),           //
		Log:           logger,                  //
		Device:        &device,                 //
		Response:      types.NewResponse(),     // Populated by all streams
		Filter:        filter,                  //
		Since:         since,                   //
		Timeout:       timeout,                 //
		Limit:         timelineLimit,           //
		Rooms:         make(map[string]string), // Populated by the PDU stream
		WantFullState: wantFullState,           //
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
