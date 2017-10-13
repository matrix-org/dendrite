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
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
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
			syncData, err = rp.appendAccountData(syncData, device.UserID, *syncReq, currentPos)
			if err != nil {
				res = httputil.LogThenError(req, err)
			} else {
				res = util.JSONResponse{
					Code: 200,
					JSON: syncData,
				}
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

type stateEventInStateResp struct {
	gomatrixserverlib.ClientEvent
	PrevContent   json.RawMessage `json:"prev_content,omitempty"`
	ReplacesState string          `json:"replaces_state,omitempty"`
}

// OnIncomingStateRequest is called when a client makes a /rooms/{roomID}/state
// request. It will fetch all the state events from the specified room and will
// append the necessary keys to them if applicable before returning them.
// Returns an error if something went wrong in the process.
// TODO: Check if the user is in the room. If not, check if the room's history
// is publicly visible. Current behaviour is returning an empty array if the
// user cannot see the room's history.
func (rp *RequestPool) OnIncomingStateRequest(req *http.Request, roomID string) util.JSONResponse {
	// TODO(#287): Auth request and handle the case where the user has left (where
	// we should return the state at the poin they left)

	stateEvents, err := rp.db.GetStateEventsForRoom(req.Context(), roomID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	resp := []stateEventInStateResp{}
	// Fill the prev_content and replaces_state keys if necessary
	for _, event := range stateEvents {
		stateEvent := stateEventInStateResp{
			ClientEvent: gomatrixserverlib.ToClientEvent(event, gomatrixserverlib.FormatAll),
		}
		var prevEventRef types.PrevEventRef
		if len(event.Unsigned()) > 0 {
			if err := json.Unmarshal(event.Unsigned(), &prevEventRef); err != nil {
				return httputil.LogThenError(req, err)
			}
			// Fills the previous state event ID if the state event replaces another
			// state event
			if len(prevEventRef.ReplacesState) > 0 {
				stateEvent.ReplacesState = prevEventRef.ReplacesState
			}
			// Fill the previous event if the state event references a previous event
			if prevEventRef.PrevContent != nil {
				stateEvent.PrevContent = prevEventRef.PrevContent
			}
		}

		resp = append(resp, stateEvent)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: resp,
	}
}

// OnIncomingStateTypeRequest is called when a client makes a
// /rooms/{roomID}/state/{type}/{statekey} request. It will look in current
// state to see if there is an event with that type and state key, if there
// is then (by default) we return the content, otherwise a 404.
func (rp *RequestPool) OnIncomingStateTypeRequest(req *http.Request, roomID string, evType, stateKey string) util.JSONResponse {
	// TODO(#287): Auth request and handle the case where the user has left (where
	// we should return the state at the poin they left)

	logger := util.GetLogger(req.Context())
	logger.WithFields(log.Fields{
		"roomID":   roomID,
		"evType":   evType,
		"stateKey": stateKey,
	}).Info("Fetching state")

	event, err := rp.db.GetStateEvent(req.Context(), roomID, evType, stateKey)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if event == nil {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("cannot find state"),
		}
	}

	stateEvent := stateEventInStateResp{
		ClientEvent: gomatrixserverlib.ToClientEvent(*event, gomatrixserverlib.FormatAll),
	}

	return util.JSONResponse{
		Code: 200,
		JSON: stateEvent.Content,
	}
}

func (rp *RequestPool) currentSyncForUser(req syncRequest, currentPos types.StreamPosition) (*types.Response, error) {
	// TODO: handle ignored users
	if req.since == types.StreamPosition(0) {
		return rp.db.CompleteSync(req.ctx, req.userID, req.limit)
	}
	return rp.db.IncrementalSync(req.ctx, req.userID, req.since, currentPos, req.limit)
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

	if req.since == types.StreamPosition(0) {
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
	dataTypes, err := rp.db.GetAccountDataInRange(req.ctx, userID, req.since, currentPos)
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
