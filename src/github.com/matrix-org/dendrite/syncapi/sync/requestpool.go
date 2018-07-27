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
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	encryptoapi "github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db        *storage.SyncServerDatabase
	accountDB *accounts.Database
	notifier  *Notifier
}

// NewRequestPool makes a new RequestPool
func NewRequestPool(db *storage.SyncServerDatabase, n *Notifier, adb *accounts.Database) *RequestPool {
	return &RequestPool{db, adb, n}
}

// OnIncomingSyncRequest is called when a client makes a /sync request. This function MUST be
// called in a dedicated goroutine for this request. This function will block the goroutine
// until a response is ready, or it times out.
func (rp *RequestPool) OnIncomingSyncRequest(req *http.Request, device *authtypes.Device, encryptDB *encryptoapi.Database) util.JSONResponse {
	// Extract values from request
	logger := util.GetLogger(req.Context())
	userID := device.UserID
	syncReq, err := newSyncRequest(req, *device)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}
	logger.WithFields(log.Fields{
		"userID":  userID,
		"since":   syncReq.since,
		"timeout": syncReq.timeout,
	}).Info("Incoming /sync request")

	currPos := rp.notifier.CurrentPosition()

	// If this is an initial sync or timeout=0 we return immediately
	if syncReq.since == nil || syncReq.timeout == 0 {
		syncData, err := rp.currentSyncForUser(*syncReq, currPos)
		if err != nil {
			return httputil.LogThenError(req, err)
		}
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

	for {
		select {
		// Wait for notifier to wake us up
		case <-userStreamListener.GetNotifyChannel(currPos):
			currPos = userStreamListener.GetStreamPosition()
		// Or for timeout to expire
		case <-timer.C:
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: types.NewResponse(currPos),
			}
		// Or for the request to be cancelled
		case <-req.Context().Done():
			return httputil.LogThenError(req, req.Context().Err())
		}

		// Note that we don't time out during calculation of sync
		// response. This ensures that we don't waste the hard work
		// of calculating the sync only to get timed out before we
		// can respond

		syncData, err := rp.currentSyncForUser(*syncReq, currPos)

		// std extension consideration
		syncData = storage.StdEXT(syncReq.ctx, rp.db, *syncData, syncReq.device.UserID, syncReq.device.ID, int64(currPos))
		syncData = KeyCountEXT(syncReq.ctx, encryptDB, *syncData, syncReq.device.UserID, syncReq.device.ID)

		if err != nil {
			return httputil.LogThenError(req, err)
		}
		if !syncData.IsEmpty() {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: syncData,
			}
		}

	}
}

func (rp *RequestPool) currentSyncForUser(req syncRequest, currentPos types.StreamPosition) (res *types.Response, err error) {
	// TODO: handle ignored users
	if req.since == nil {
		res, err = rp.db.CompleteSync(req.ctx, req.device.UserID, req.limit)
	} else {
		res, err = rp.db.IncrementalSync(req.ctx, req.device, *req.since, currentPos, req.limit)
	}

	if err != nil {
		return
	}

	res, err = rp.appendAccountData(res, req.device.UserID, req, currentPos)
	return
}

func (rp *RequestPool) appendAccountData(
	data *types.Response, userID string, req syncRequest, currentPos types.StreamPosition,
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

	// Sync is not initial, get all account data since the latest sync
	dataTypes, err := rp.db.GetAccountDataInRange(req.ctx, userID, *req.since, currentPos)
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
			evs, err := rp.accountDB.GetAccountDataByType(
				req.ctx, localpart, roomID, dataType,
			)
			if err != nil {
				return nil, err
			}
			events = append(events, evs...)
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
