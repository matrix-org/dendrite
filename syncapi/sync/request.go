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
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

const defaultSyncTimeout = time.Duration(0)
const defaultTimelineLimit = 20

// syncRequest represents a /sync request, with sensible defaults/sanity checks applied.
type syncRequest struct {
	ctx           context.Context
	device        authtypes.Device
	limit         int
	timeout       time.Duration
	since         *types.SyncPosition // nil means that no since token was supplied
	wantFullState bool
	log           *log.Entry
}

func newSyncRequest(req *http.Request, device authtypes.Device) (*syncRequest, error) {
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
		device:        device,
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

// getSyncStreamPosition tries to parse a 'since' token taken from the API to a
// types.SyncPosition. If the string is empty then (nil, nil) is returned.
// There are two forms of tokens: The full length form containing all PDU and EDU
// positions separated by "_", and the short form containing only the PDU
// position. Short form can be used for, e.g., `prev_batch` tokens.
func getSyncStreamPosition(since string) (*types.SyncPosition, error) {
	if since == "" {
		return nil, nil
	}

	posStrings := strings.Split(since, "_")
	if len(posStrings) != 2 && len(posStrings) != 1 {
		// A token can either be full length or short (PDU-only).
		return nil, errors.New("malformed batch token")
	}

	positions := make([]int64, len(posStrings))
	for i, posString := range posStrings {
		pos, err := strconv.ParseInt(posString, 10, 64)
		if err != nil {
			return nil, err
		}
		positions[i] = pos
	}

	if len(positions) == 2 {
		// Full length token; construct SyncPosition with every entry in
		// `positions`. These entries must have the same order with the fields
		// in struct SyncPosition, so we disable the govet check below.
		return &types.SyncPosition{ //nolint:govet
			positions[0], positions[1],
		}, nil
	} else {
		// Token with PDU position only
		return &types.SyncPosition{
			PDUPosition: positions[0],
		}, nil
	}
}
