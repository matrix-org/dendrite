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
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

const defaultSyncTimeout = time.Duration(30) * time.Second
const defaultTimelineLimit = 20

var (
	// ErrNotStreamToken is returned if a pagination token isn't of type
	// types.PaginationTokenTypeStream
	ErrNotStreamToken = fmt.Errorf("The provided pagination token has the wrong prefix (should be s)")
)

// syncRequest represents a /sync request, with sensible defaults/sanity checks applied.
type syncRequest struct {
	ctx           context.Context
	device        authtypes.Device
	limit         int
	timeout       time.Duration
	since         *types.StreamPosition // nil means that no since token was supplied
	wantFullState bool
	log           *log.Entry
}

func newSyncRequest(req *http.Request, device authtypes.Device) (*syncRequest, error) {
	timeout := getTimeout(req.URL.Query().Get("timeout"))
	fullState := req.URL.Query().Get("full_state")
	wantFullState := fullState != "" && fullState != "false"
	since, err := getPaginationToken(req.URL.Query().Get("since"))
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

// getPaginationToken tries to parse a 'since' token taken from the API to a
// pagination token. If the string is empty then (nil, nil) is returned.
// Returns an error if the parsed token's type isn't types.PaginationTokenTypeStream.
func getPaginationToken(since string) (*types.StreamPosition, error) {
	if since == "" {
		return nil, nil
	}
	p, err := types.NewPaginationTokenFromString(since)
	if err != nil {
		return nil, err
	}
	if p.Type != types.PaginationTokenTypeStream {
		return nil, ErrNotStreamToken
	}
	return &(p.Position), nil
}
