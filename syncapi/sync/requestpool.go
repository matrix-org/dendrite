// Copyright 2017 Vector Creations Ltd
// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/internal"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db       storage.Database
	userAPI  userapi.UserInternalAPI
	notifier *Notifier
	keyAPI   keyapi.KeyInternalAPI
	rsAPI    roomserverAPI.RoomserverInternalAPI
}

// NewRequestPool makes a new RequestPool
func NewRequestPool(
	db storage.Database, n *Notifier, userAPI userapi.UserInternalAPI, keyAPI keyapi.KeyInternalAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) *RequestPool {
	return &RequestPool{db, userAPI, n, keyAPI, rsAPI}
}

// OnIncomingSyncRequest is called when a client makes a /sync request. This function MUST be
// called in a dedicated goroutine for this request. This function will block the goroutine
// until a response is ready, or it times out.
func (rp *RequestPool) OnIncomingSyncRequest(req *http.Request, device *userapi.Device) util.JSONResponse {
	var syncData *types.Response

	// Extract values from request
	syncReq, err := newSyncRequest(req, *device, rp.db)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}

	logger := util.GetLogger(req.Context()).WithFields(log.Fields{
		"user_id":   device.UserID,
		"device_id": device.ID,
		"since":     syncReq.since,
		"timeout":   syncReq.timeout,
		"limit":     syncReq.limit,
	})

	currPos := rp.notifier.CurrentPosition()

	if rp.shouldReturnImmediately(syncReq) {
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

func (rp *RequestPool) OnIncomingKeyChangeRequest(req *http.Request, device *userapi.Device) util.JSONResponse {
	from := req.URL.Query().Get("from")
	to := req.URL.Query().Get("to")
	if from == "" || to == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidArgumentValue("missing ?from= or ?to="),
		}
	}
	fromToken, err := types.NewStreamTokenFromString(from)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidArgumentValue("bad 'from' value"),
		}
	}
	toToken, err := types.NewStreamTokenFromString(to)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidArgumentValue("bad 'to' value"),
		}
	}
	// work out room joins/leaves
	res, err := rp.db.IncrementalSync(
		req.Context(), types.NewResponse(), *device, fromToken, toToken, 10, false,
	)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("Failed to IncrementalSync")
		return jsonerror.InternalServerError()
	}

	res, err = rp.appendDeviceLists(res, device.UserID, fromToken, toToken)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("Failed to appendDeviceLists info")
		return jsonerror.InternalServerError()
	}
	return util.JSONResponse{
		Code: 200,
		JSON: struct {
			Changed []string `json:"changed"`
			Left    []string `json:"left"`
		}{
			Changed: res.DeviceLists.Changed,
			Left:    res.DeviceLists.Left,
		},
	}
}

// nolint:gocyclo
func (rp *RequestPool) currentSyncForUser(req syncRequest, latestPos types.StreamingToken) (*types.Response, error) {
	res := types.NewResponse()

	since := types.NewStreamToken(0, 0, nil)
	if req.since != nil {
		since = *req.since
	}

	// See if we have any new tasks to do for the send-to-device messaging.
	events, updates, deletions, err := rp.db.SendToDeviceUpdatesForSync(req.ctx, req.device.UserID, req.device.ID, since)
	if err != nil {
		return nil, fmt.Errorf("rp.db.SendToDeviceUpdatesForSync: %w", err)
	}

	// TODO: handle ignored users
	if req.since == nil {
		res, err = rp.db.CompleteSync(req.ctx, res, req.device, req.limit)
		if err != nil {
			return res, fmt.Errorf("rp.db.CompleteSync: %w", err)
		}
	} else {
		res, err = rp.db.IncrementalSync(req.ctx, res, req.device, *req.since, latestPos, req.limit, req.wantFullState)
		if err != nil {
			return res, fmt.Errorf("rp.db.IncrementalSync: %w", err)
		}
	}

	accountDataFilter := gomatrixserverlib.DefaultEventFilter() // TODO: use filter provided in req instead
	res, err = rp.appendAccountData(res, req.device.UserID, req, latestPos.PDUPosition(), &accountDataFilter)
	if err != nil {
		return res, fmt.Errorf("rp.appendAccountData: %w", err)
	}
	res, err = rp.appendDeviceLists(res, req.device.UserID, since, latestPos)
	if err != nil {
		return res, fmt.Errorf("rp.appendDeviceLists: %w", err)
	}
	err = internal.DeviceOTKCounts(req.ctx, rp.keyAPI, req.device.UserID, req.device.ID, res)
	if err != nil {
		return res, fmt.Errorf("internal.DeviceOTKCounts: %w", err)
	}

	// Before we return the sync response, make sure that we take action on
	// any send-to-device database updates or deletions that we need to do.
	// Then add the updates into the sync response.
	if len(updates) > 0 || len(deletions) > 0 {
		// Handle the updates and deletions in the database.
		err = rp.db.CleanSendToDeviceUpdates(context.Background(), updates, deletions, since)
		if err != nil {
			return res, fmt.Errorf("rp.db.CleanSendToDeviceUpdates: %w", err)
		}
	}
	if len(events) > 0 {
		// Add the updates into the sync response.
		for _, event := range events {
			res.ToDevice.Events = append(res.ToDevice.Events, event.SendToDeviceEvent)
		}

		// Get the next_batch from the sync response and increase the
		// EDU counter.
		if pos, perr := types.NewStreamTokenFromString(res.NextBatch); perr == nil {
			pos.Positions[1]++
			res.NextBatch = pos.String()
		}
	}

	return res, err
}

func (rp *RequestPool) appendDeviceLists(
	data *types.Response, userID string, since, to types.StreamingToken,
) (*types.Response, error) {
	_, err := internal.DeviceListCatchup(context.Background(), rp.keyAPI, rp.rsAPI, userID, data, since, to)
	if err != nil {
		return nil, fmt.Errorf("internal.DeviceListCatchup: %w", err)
	}

	return data, nil
}

// nolint:gocyclo
func (rp *RequestPool) appendAccountData(
	data *types.Response, userID string, req syncRequest, currentPos types.StreamPosition,
	accountDataFilter *gomatrixserverlib.EventFilter,
) (*types.Response, error) {
	// TODO: Account data doesn't have a sync position of its own, meaning that
	// account data might be sent multiple time to the client if multiple account
	// data keys were set between two message. This isn't a huge issue since the
	// duplicate data doesn't represent a huge quantity of data, but an optimisation
	// here would be making sure each data is sent only once to the client.
	if req.since == nil {
		// If this is the initial sync, we don't need to check if a data has
		// already been sent. Instead, we send the whole batch.
		dataReq := &userapi.QueryAccountDataRequest{
			UserID: userID,
		}
		dataRes := &userapi.QueryAccountDataResponse{}
		if err := rp.userAPI.QueryAccountData(req.ctx, dataReq, dataRes); err != nil {
			return nil, err
		}
		for datatype, databody := range dataRes.GlobalAccountData {
			data.AccountData.Events = append(
				data.AccountData.Events,
				gomatrixserverlib.ClientEvent{
					Type:    datatype,
					Content: gomatrixserverlib.RawJSON(databody),
				},
			)
		}
		for r, j := range data.Rooms.Join {
			for datatype, databody := range dataRes.RoomAccountData[r] {
				j.AccountData.Events = append(
					j.AccountData.Events,
					gomatrixserverlib.ClientEvent{
						Type:    datatype,
						Content: gomatrixserverlib.RawJSON(databody),
					},
				)
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
		return nil, fmt.Errorf("rp.db.GetAccountDataInRange: %w", err)
	}

	if len(dataTypes) == 0 {
		// TODO: this fixes the sytest but is it the right thing to do?
		dataTypes[""] = []string{"m.push_rules"}
	}

	// Iterate over the rooms
	for roomID, dataTypes := range dataTypes {
		// Request the missing data from the database
		for _, dataType := range dataTypes {
			dataReq := userapi.QueryAccountDataRequest{
				UserID:   userID,
				RoomID:   roomID,
				DataType: dataType,
			}
			dataRes := userapi.QueryAccountDataResponse{}
			err = rp.userAPI.QueryAccountData(req.ctx, &dataReq, &dataRes)
			if err != nil {
				continue
			}
			if roomID == "" {
				if globalData, ok := dataRes.GlobalAccountData[dataType]; ok {
					data.AccountData.Events = append(
						data.AccountData.Events,
						gomatrixserverlib.ClientEvent{
							Type:    dataType,
							Content: gomatrixserverlib.RawJSON(globalData),
						},
					)
				}
			} else {
				if roomData, ok := dataRes.RoomAccountData[roomID][dataType]; ok {
					joinData := data.Rooms.Join[roomID]
					joinData.AccountData.Events = append(
						joinData.AccountData.Events,
						gomatrixserverlib.ClientEvent{
							Type:    dataType,
							Content: gomatrixserverlib.RawJSON(roomData),
						},
					)
					data.Rooms.Join[roomID] = joinData
				}
			}
		}
	}

	return data, nil
}

// shouldReturnImmediately returns whether the /sync request is an initial sync,
// or timeout=0, or full_state=true, in any of the cases the request should
// return immediately.
func (rp *RequestPool) shouldReturnImmediately(syncReq *syncRequest) bool {
	if syncReq.since == nil || syncReq.timeout == 0 || syncReq.wantFullState {
		return true
	}
	waiting, werr := rp.db.SendToDeviceUpdatesWaiting(context.TODO(), syncReq.device.UserID, syncReq.device.ID)
	return werr == nil && waiting
}
