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
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
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
	// TODO: read from stored filters too
	filter := gomatrixserverlib.DefaultFilter()
	if since.IsEmpty() {
		// Send as much account data down for complete syncs as possible
		// by default, otherwise clients do weird things while waiting
		// for the rest of the data to trickle down.
		filter.AccountData.Limit = math.MaxInt
		filter.Room.AccountData.Limit = math.MaxInt
	}
	filterQuery := req.URL.Query().Get("filter")
	if filterQuery != "" {
		if filterQuery[0] == '{' {
			// Parse the filter from the query string
			if err := json.Unmarshal([]byte(filterQuery), &filter); err != nil {
				return nil, fmt.Errorf("json.Unmarshal: %w", err)
			}
		} else {
			// Try to load the filter from the database
			localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
			if err != nil {
				util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
				return nil, fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
			}
			if err := syncDB.GetFilter(req.Context(), &filter, localpart, filterQuery); err != nil && err != sql.ErrNoRows {
				util.GetLogger(req.Context()).WithError(err).Error("syncDB.GetFilter failed")
				return nil, fmt.Errorf("syncDB.GetFilter: %w", err)
			}
		}
	}

	logger := util.GetLogger(req.Context()).WithFields(logrus.Fields{
		"user_id":   device.UserID,
		"device_id": device.ID,
		"since":     since,
		"timeout":   timeout,
		"limit":     filter.Room.Timeline.Limit,
	})

	return &types.SyncRequest{
		Context:       req.Context(),           //
		Log:           logger,                  //
		Device:        &device,                 //
		Response:      types.NewResponse(),     // Populated by all streams
		Filter:        filter,                  //
		Since:         since,                   //
		Timeout:       timeout,                 //
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
