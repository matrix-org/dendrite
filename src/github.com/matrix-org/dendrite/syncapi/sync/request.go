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
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/gomatrix"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

const defaultSyncTimeout = time.Duration(30) * time.Second

// syncRequest represents a /sync request, with sensible defaults/sanity checks applied.
type syncRequest struct {
	ctx           context.Context
	device        authtypes.Device
	timeout       time.Duration
	since         *types.StreamPosition // nil means that no since token was supplied
	wantFullState bool
	log           *log.Entry
	filter        gomatrix.Filter
}

func newSyncRequest(req *http.Request, device authtypes.Device) (*syncRequest, error) {
	timeout := getTimeout(req.URL.Query().Get("timeout"))
	fullState := req.URL.Query().Get("full_state")
	wantFullState := fullState != "" && fullState != "false"
	since, err := getSyncStreamPosition(req.URL.Query().Get("since"))
	if err != nil {
		return nil, err
	}

	filterStr := req.URL.Query().Get("filter")
	var filter gomatrix.Filter
	if filterStr != "" {
		if filterStr[0] == '{' {
			// Inline filter
			filter = gomatrix.DefaultFilter()
			err = json.Unmarshal([]byte(filterStr), &filter)
			if err != nil {
				return nil, err
			}
			err = filter.Validate()
			if err != nil {
				return nil, err
			}
		} else {
			// Filter ID
			// TODO retrieve filter from DB
			return nil, errors.New("Filter ID retrieval not implemented")
		}
	}

	// TODO: Additional query params: set_presence
	return &syncRequest{
		ctx:           req.Context(),
		device:        device,
		timeout:       timeout,
		since:         since,
		wantFullState: wantFullState,
		log:           util.GetLogger(req.Context()),
		filter:        filter,
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

// getSyncStreamPosition tries to parse a 'since' token taken from the API to a
// stream position. If the string is empty then (nil, nil) is returned.
func getSyncStreamPosition(since string) (*types.StreamPosition, error) {
	if since == "" {
		return nil, nil
	}
	i, err := strconv.Atoi(since)
	if err != nil {
		return nil, err
	}
	token := types.StreamPosition(i)
	return &token, nil
}
