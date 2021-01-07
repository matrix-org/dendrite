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
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/internal"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/streams"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db       storage.Database
	cfg      *config.SyncAPI
	userAPI  userapi.UserInternalAPI
	keyAPI   keyapi.KeyInternalAPI
	rsAPI    roomserverAPI.RoomserverInternalAPI
	lastseen sync.Map
	streams  *streams.Streams
}

// NewRequestPool makes a new RequestPool
func NewRequestPool(
	db storage.Database, cfg *config.SyncAPI,
	userAPI userapi.UserInternalAPI, keyAPI keyapi.KeyInternalAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	streams *streams.Streams,
) *RequestPool {
	rp := &RequestPool{
		db:       db,
		cfg:      cfg,
		userAPI:  userAPI,
		keyAPI:   keyAPI,
		rsAPI:    rsAPI,
		lastseen: sync.Map{},
		streams:  streams,
	}
	go rp.cleanLastSeen()
	return rp
}

func (rp *RequestPool) cleanLastSeen() {
	for {
		rp.lastseen.Range(func(key interface{}, _ interface{}) bool {
			rp.lastseen.Delete(key)
			return true
		})
		time.Sleep(time.Minute)
	}
}

func (rp *RequestPool) updateLastSeen(req *http.Request, device *userapi.Device) {
	if _, ok := rp.lastseen.LoadOrStore(device.UserID+device.ID, struct{}{}); ok {
		return
	}

	remoteAddr := req.RemoteAddr
	if rp.cfg.RealIPHeader != "" {
		if header := req.Header.Get(rp.cfg.RealIPHeader); header != "" {
			// TODO: Maybe this isn't great but it will satisfy both X-Real-IP
			// and X-Forwarded-For (which can be a list where the real client
			// address is the first listed address). Make more intelligent?
			addresses := strings.Split(header, ",")
			if ip := net.ParseIP(addresses[0]); ip != nil {
				remoteAddr = addresses[0]
			}
		}
	}

	lsreq := &userapi.PerformLastSeenUpdateRequest{
		UserID:     device.UserID,
		DeviceID:   device.ID,
		RemoteAddr: remoteAddr,
	}
	lsres := &userapi.PerformLastSeenUpdateResponse{}
	go rp.userAPI.PerformLastSeenUpdate(req.Context(), lsreq, lsres) // nolint:errcheck

	rp.lastseen.Store(device.UserID+device.ID, time.Now())
}

func init() {
	prometheus.MustRegister(
		activeSyncRequests, waitingSyncRequests,
	)
}

var activeSyncRequests = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "syncapi",
		Name:      "active_sync_requests",
		Help:      "The number of sync requests that are active right now",
	},
)

var waitingSyncRequests = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "syncapi",
		Name:      "waiting_sync_requests",
		Help:      "The number of sync requests that are waiting to be woken by a notifier",
	},
)

// OnIncomingSyncRequest is called when a client makes a /sync request. This function MUST be
// called in a dedicated goroutine for this request. This function will block the goroutine
// until a response is ready, or it times out.
func (rp *RequestPool) OnIncomingSyncRequest(req *http.Request, device *userapi.Device) util.JSONResponse {
	// Extract values from request
	syncReq, err := newSyncRequest(req, *device, rp.db)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}

	activeSyncRequests.Inc()
	defer activeSyncRequests.Dec()

	rp.updateLastSeen(req, device)

	waitingSyncRequests.Inc()
	defer waitingSyncRequests.Dec()

	if !rp.shouldReturnImmediately(syncReq) {
		timer := time.NewTimer(syncReq.Timeout) // case of timeout=0 is handled above
		defer timer.Stop()

		// Use a subcontext so that we don't keep the StreamNotifyAfter
		// goroutines alive any longer than they really need to be.
		waitctx, waitcancel := context.WithCancel(syncReq.Context)
		giveup := func() util.JSONResponse {
			waitcancel()
			syncReq.Response.NextBatch = syncReq.Since
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: syncReq.Response,
			}
		}

		select {
		case <-waitctx.Done(): // Caller gave up
			return giveup()

		case <-timer.C: // Timeout reached
			return giveup()

		case <-rp.streams.PDUStreamProvider.NotifyAfter(waitctx, device, syncReq.Since.PDUPosition):
		case <-rp.streams.TypingStreamProvider.NotifyAfter(waitctx, device, syncReq.Since.TypingPosition):
		case <-rp.streams.ReceiptStreamProvider.NotifyAfter(waitctx, device, syncReq.Since.ReceiptPosition):
		case <-rp.streams.InviteStreamProvider.NotifyAfter(waitctx, device, syncReq.Since.InvitePosition):
		case <-rp.streams.SendToDeviceStreamProvider.NotifyAfter(waitctx, device, syncReq.Since.SendToDevicePosition):
		case <-rp.streams.AccountDataStreamProvider.NotifyAfter(waitctx, device, syncReq.Since.AccountDataPosition):
		case <-rp.streams.DeviceListStreamProvider.NotifyAfter(waitctx, device, syncReq.Since.DeviceListPosition):
		}

		syncReq.Log.Println("Responding to sync after wakeup")
		waitcancel()
	} else {
		syncReq.Log.Println("Responding to sync immediately")
	}

	if syncReq.Since.IsEmpty() {
		// Complete sync
		syncReq.Response.NextBatch = types.StreamingToken{
			PDUPosition: rp.streams.PDUStreamProvider.CompleteSync(
				syncReq.Context, syncReq,
			),
			TypingPosition: rp.streams.TypingStreamProvider.CompleteSync(
				syncReq.Context, syncReq,
			),
			ReceiptPosition: rp.streams.ReceiptStreamProvider.CompleteSync(
				syncReq.Context, syncReq,
			),
			InvitePosition: rp.streams.InviteStreamProvider.CompleteSync(
				syncReq.Context, syncReq,
			),
			SendToDevicePosition: rp.streams.SendToDeviceStreamProvider.CompleteSync(
				syncReq.Context, syncReq,
			),
			AccountDataPosition: rp.streams.AccountDataStreamProvider.CompleteSync(
				syncReq.Context, syncReq,
			),
			DeviceListPosition: rp.streams.DeviceListStreamProvider.CompleteSync(
				syncReq.Context, syncReq,
			),
		}
	} else {
		// Incremental sync
		syncReq.Response.NextBatch = types.StreamingToken{
			PDUPosition: rp.streams.PDUStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.PDUPosition,                                    // from
				rp.streams.PDUStreamProvider.LatestPosition(syncReq.Context), // to
			),
			TypingPosition: rp.streams.TypingStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.TypingPosition,                                    // from
				rp.streams.TypingStreamProvider.LatestPosition(syncReq.Context), // to
			),
			ReceiptPosition: rp.streams.ReceiptStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.ReceiptPosition,                                    // from
				rp.streams.ReceiptStreamProvider.LatestPosition(syncReq.Context), // to
			),
			InvitePosition: rp.streams.InviteStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.InvitePosition,                                    // from
				rp.streams.InviteStreamProvider.LatestPosition(syncReq.Context), // to
			),
			SendToDevicePosition: rp.streams.SendToDeviceStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.SendToDevicePosition,                                    // from
				rp.streams.SendToDeviceStreamProvider.LatestPosition(syncReq.Context), // to
			),
			AccountDataPosition: rp.streams.AccountDataStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.AccountDataPosition,                                    // from
				rp.streams.AccountDataStreamProvider.LatestPosition(syncReq.Context), // to
			),
			DeviceListPosition: rp.streams.DeviceListStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.DeviceListPosition,                                    // from
				rp.streams.DeviceListStreamProvider.LatestPosition(syncReq.Context), // to
			),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: syncReq.Response,
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
	/*
		res, err := rp.db.IncrementalSync(
			req.Context(), types.NewResponse(), *device, fromToken, toToken, 10, false,
		)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("Failed to IncrementalSync")
			return jsonerror.InternalServerError()
		}
	*/
	res := types.NewResponse()
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
/*
func (rp *RequestPool) currentSyncForUser(req syncRequest, latestPos types.StreamingToken) (*types.Response, error) {
	res := types.NewResponse()

	// TODO: handle ignored users
	if req.since.IsEmpty() {
		res, err = rp.db.CompleteSync(req.ctx, res, req.device, req.limit)
		if err != nil {
			return res, fmt.Errorf("rp.db.CompleteSync: %w", err)
		}
	} else {
		res, err = rp.db.IncrementalSync(req.ctx, res, req.device, req.since, latestPos, req.limit, req.wantFullState)
		if err != nil {
			return res, fmt.Errorf("rp.db.IncrementalSync: %w", err)
		}
	}

	accountDataFilter := gomatrixserverlib.DefaultEventFilter() // TODO: use filter provided in req instead
	res, err = rp.appendAccountData(res, req.device.UserID, req, latestPos.PDUPosition, &accountDataFilter)
	if err != nil {
		return res, fmt.Errorf("rp.appendAccountData: %w", err)
	}
	res, err = rp.appendDeviceLists(res, req.device.UserID, req.since, latestPos)
	if err != nil {
		return res, fmt.Errorf("rp.appendDeviceLists: %w", err)
	}
	err = internal.DeviceOTKCounts(req.ctx, rp.keyAPI, req.device.UserID, req.device.ID, res)
	if err != nil {
		return res, fmt.Errorf("internal.DeviceOTKCounts: %w", err)
	}

	return res, err
}
*/

func (rp *RequestPool) appendDeviceLists(
	data *types.Response, userID string, since, to types.StreamingToken,
) (*types.Response, error) {
	_, err := internal.DeviceListCatchup(context.Background(), rp.keyAPI, rp.rsAPI, userID, data, since, to)
	if err != nil {
		return nil, fmt.Errorf("internal.DeviceListCatchup: %w", err)
	}

	return data, nil
}

/*
func (rp *RequestPool) appendAccountData(
	data *types.Response, userID string, req syncRequest, currentPos types.StreamPosition,
	accountDataFilter *gomatrixserverlib.EventFilter,
) (*types.Response, error) {
	// TODO: Account data doesn't have a sync position of its own, meaning that
	// account data might be sent multiple time to the client if multiple account
	// data keys were set between two message. This isn't a huge issue since the
	// duplicate data doesn't represent a huge quantity of data, but an optimisation
	// here would be making sure each data is sent only once to the client.
	if req.since.IsEmpty() {
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
		From: req.since.PDUPosition,
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
*/

// shouldReturnImmediately returns whether the /sync request is an initial sync,
// or timeout=0, or full_state=true, in any of the cases the request should
// return immediately.
func (rp *RequestPool) shouldReturnImmediately(syncReq *types.SyncRequest) bool {
	if syncReq.Since.IsEmpty() || syncReq.Timeout == 0 || syncReq.WantFullState {
		return true
	}
	waiting, werr := rp.db.SendToDeviceUpdatesWaiting(context.TODO(), syncReq.Device.UserID, syncReq.Device.ID)
	return werr == nil && waiting
}
