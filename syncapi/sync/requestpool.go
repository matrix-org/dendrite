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

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db        storage.Database
	accountDB accounts.Database
	notifier  *Notifier
}

// NewRequestPool makes a new RequestPool
func NewRequestPool(db storage.Database, n *Notifier, adb accounts.Database) *RequestPool {
	return &RequestPool{db, adb, n}
}

// OnIncomingSyncRequest is called when a client makes a /sync request. This function MUST be
// called in a dedicated goroutine for this request. This function will block the goroutine
// until a response is ready, or it times out.
func (rp *RequestPool) OnIncomingSyncRequest(req *http.Request, device *authtypes.Device) util.JSONResponse {
	var syncData *types.Response

	// Extract values from request
	userID := device.UserID
	syncReq, err := newSyncRequest(req, *device)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}
	logger := util.GetLogger(req.Context()).WithFields(log.Fields{
		"userID":  userID,
		"since":   syncReq.since,
		"timeout": syncReq.timeout,
		"limit":   syncReq.limit,
	})

	currPos := rp.notifier.CurrentPosition()

	if shouldReturnImmediately(syncReq) {
		syncData, err = rp.currentSyncForUser(*syncReq, currPos)
		if err != nil {
			logger.WithError(err).Error("rp.currentSyncForUser failed")
			return jsonerror.InternalServerError()
		}
		logger.WithField("next", syncData.NextBatch).Info("Responding immediately")
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: syncData,
		}
	}

	// Otherwise, we wait for the notifier to tell us if something *may* have
	// happened. We loop in case it turns out that nothing did happen.

	timer := time.NewTimer(syncReq.timeout) // case of timeout=0 is handled above
	defer timer.Stop()

	userStreamListener := rp.notifier.GetListener(*syncReq)
	defer userStreamListener.Close()

	// We need the loop in case userStreamListener wakes up even if there isn't
	// anything to send down. In this case, we'll jump out of the select but
	// don't want to send anything back until we get some actual content to
	// respond with, so we skip the return an go back to waiting for content to
	// be sent down or the request timing out.
	var hasTimedOut bool
	sincePos := *syncReq.since
	for {
		select {
		// Wait for notifier to wake us up
		case <-userStreamListener.GetNotifyChannel(sincePos):
			currPos = userStreamListener.GetSyncPosition()
			sincePos = currPos
		// Or for timeout to expire
		case <-timer.C:
			// We just need to ensure we get out of the select after reaching the
			// timeout, but there's nothing specific we want to do in this case
			// apart from that, so we do nothing except stating we're timing out
			// and need to respond.
			hasTimedOut = true
		// Or for the request to be cancelled
		case <-req.Context().Done():
			logger.WithError(err).Error("request cancelled")
			return jsonerror.InternalServerError()
		}

		// Note that we don't time out during calculation of sync
		// response. This ensures that we don't waste the hard work
		// of calculating the sync only to get timed out before we
		// can respond

		syncData, err = rp.currentSyncForUser(*syncReq, currPos)
		if err != nil {
			logger.WithError(err).Error("rp.currentSyncForUser failed")
			return jsonerror.InternalServerError()
		}

		if !syncData.IsEmpty() || hasTimedOut {
			logger.WithField("next", syncData.NextBatch).WithField("timed_out", hasTimedOut).Info("Responding")
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: syncData,
			}
		}
	}
}

func (rp *RequestPool) currentSyncForUser(req syncRequest, latestPos types.StreamingToken) (res *types.Response, err error) {
	// TODO: handle ignored users
	if req.since == nil {
		res, err = rp.db.CompleteSync(req.ctx, req.device.UserID, req.limit)
	} else {
		res, err = rp.db.IncrementalSync(req.ctx, req.device, *req.since, latestPos, req.limit, req.wantFullState)
	}

	if err != nil {
		return
	}

	accountDataFilter := gomatrixserverlib.DefaultEventFilter() // TODO: use filter provided in req instead
	res, err = rp.appendAccountData(res, req.device.UserID, req, latestPos.PDUPosition(), &accountDataFilter)
	return
}

func (rp *RequestPool) appendAccountData(
	data *types.Response, userID string, req syncRequest, currentPos types.StreamPosition,
	accountDataFilter *gomatrixserverlib.EventFilter,
) (*types.Response, error) {
	// TODO: Account data doesn't have a sync position of its own, meaning that
	// account data might be sent multiple time to the client if multiple account
	// data keys were set between two message. This isn't a huge issue since the
	// duplicate data doesn't represent a huge quantity of data, but an optimisation
	// here would be making sure each data is sent only once to the client.
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return nil, err
	}

	if req.since == nil {
		// If this is the initial sync, we don't need to check if a data has
		// already been sent. Instead, we send the whole batch.
		var global []gomatrixserverlib.ClientEvent
		var rooms map[string][]gomatrixserverlib.ClientEvent
		global, rooms, err = rp.accountDB.GetAccountData(req.ctx, localpart)
		if err != nil {
			return nil, err
		}
		data.AccountData.Events = global

		for r, j := range data.Rooms.Join {
			if len(rooms[r]) > 0 {
				j.AccountData.Events = rooms[r]
				data.Rooms.Join[r] = j
			}
		}

		return data, nil
	}

	r := types.Range{
		From: req.since.PDUPosition(),
		To:   currentPos,
	}
	// If both positions are the same, it means that the data was saved after the
	// latest room event. In that case, we need to decrement the old position as
	// results are exclusive of Low.
	if r.Low() == r.High() {
		r.From--
	}

	// Sync is not initial, get all account data since the latest sync
	dataTypes, err := rp.db.GetAccountDataInRange(
		req.ctx, userID, r, accountDataFilter,
	)
	if err != nil {
		return nil, err
	}

	if len(dataTypes) == 0 {
		return data, nil
	}

	// Iterate over the rooms
	for roomID, dataTypes := range dataTypes {
		events := []gomatrixserverlib.ClientEvent{}
		// Request the missing data from the database
		for _, dataType := range dataTypes {
			event, err := rp.accountDB.GetAccountDataByType(
				req.ctx, localpart, roomID, dataType,
			)
			if err != nil {
				return nil, err
			}
			events = append(events, *event)
		}

		// Append the data to the response
		if len(roomID) > 0 {
			jr := data.Rooms.Join[roomID]
			jr.AccountData.Events = events
			data.Rooms.Join[roomID] = jr
		} else {
			data.AccountData.Events = events
		}
	}

	return data, nil
}

// shouldReturnImmediately returns whether the /sync request is an initial sync,
// or timeout=0, or full_state=true, in any of the cases the request should
// return immediately.
func shouldReturnImmediately(syncReq *syncRequest) bool {
	return syncReq.since == nil || syncReq.timeout == 0 || syncReq.wantFullState
}
