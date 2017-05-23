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
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/util"
)

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db       *storage.SyncServerDatabase
	notifier *Notifier
}

// NewRequestPool makes a new RequestPool
func NewRequestPool(db *storage.SyncServerDatabase, n *Notifier) *RequestPool {
	return &RequestPool{db, n}
}

// OnIncomingSyncRequest is called when a client makes a /sync request. This function MUST be
// called in a dedicated goroutine for this request. This function will block the goroutine
// until a response is ready, or it times out.
func (rp *RequestPool) OnIncomingSyncRequest(req *http.Request, device *authtypes.Device) util.JSONResponse {
	// Extract values from request
	logger := util.GetLogger(req.Context())
	userID := device.UserID
	syncReq, err := newSyncRequest(req, userID)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}
	logger.WithFields(log.Fields{
		"userID":  userID,
		"since":   syncReq.since,
		"timeout": syncReq.timeout,
	}).Info("Incoming /sync request")

	// Fork off 2 goroutines: one to do the work, and one to serve as a timeout.
	// Whichever returns first is the one we will serve back to the client.
	timeoutChan := make(chan struct{})
	timer := time.AfterFunc(syncReq.timeout, func() {
		close(timeoutChan) // signal that the timeout has expired
	})

	done := make(chan util.JSONResponse)
	go func() {
		currentPos := rp.notifier.WaitForEvents(*syncReq)
		// We stop the timer BEFORE calculating the response so the cpu work
		// done to calculate the response is not timed. This stops us from
		// doing lots of work then timing out and sending back an empty response.
		timer.Stop()
		syncData, err := rp.currentSyncForUser(*syncReq, currentPos)
		var res util.JSONResponse
		if err != nil {
			res = httputil.LogThenError(req, err)
		} else {
			res = util.JSONResponse{
				Code: 200,
				JSON: syncData,
			}
		}
		done <- res
		close(done)
	}()

	select {
	case <-timeoutChan: // timeout fired
		return util.JSONResponse{
			Code: 200,
			JSON: types.NewResponse(syncReq.since),
		}
	case res := <-done: // received a response
		return res
	}
}

func (rp *RequestPool) currentSyncForUser(req syncRequest, currentPos types.StreamPosition) (*types.Response, error) {
	// TODO: handle ignored users
	if req.since == types.StreamPosition(0) {
		return rp.db.CompleteSync(req.userID, req.limit)
	}
	return rp.db.IncrementalSync(req.userID, req.since, currentPos, req.limit)
}
