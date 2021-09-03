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
	"github.com/matrix-org/dendrite/syncapi/notifier"
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
	Notifier *notifier.Notifier
}

// NewRequestPool makes a new RequestPool
func NewRequestPool(
	db storage.Database, cfg *config.SyncAPI,
	userAPI userapi.UserInternalAPI, keyAPI keyapi.KeyInternalAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	streams *streams.Streams, notifier *notifier.Notifier,
) *RequestPool {
	rp := &RequestPool{
		db:       db,
		cfg:      cfg,
		userAPI:  userAPI,
		keyAPI:   keyAPI,
		rsAPI:    rsAPI,
		lastseen: sync.Map{},
		streams:  streams,
		Notifier: notifier,
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

	currentPos := rp.Notifier.CurrentPosition()

	if !rp.shouldReturnImmediately(syncReq, currentPos) {
		timer := time.NewTimer(syncReq.Timeout) // case of timeout=0 is handled above
		defer timer.Stop()

		userStreamListener := rp.Notifier.GetListener(*syncReq)
		defer userStreamListener.Close()

		giveup := func() util.JSONResponse {
			syncReq.Response.NextBatch = syncReq.Since
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: syncReq.Response,
			}
		}

		select {
		case <-syncReq.Context.Done(): // Caller gave up
			return giveup()

		case <-timer.C: // Timeout reached
			return giveup()

		case <-userStreamListener.GetNotifyChannel(syncReq.Since):
			syncReq.Log.Debugln("Responding to sync after wake-up")
			currentPos.ApplyUpdates(userStreamListener.GetSyncPosition())
		}
	} else {
		syncReq.Log.Debugln("Responding to sync immediately")
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
				syncReq.Since.PDUPosition, currentPos.PDUPosition,
			),
			TypingPosition: rp.streams.TypingStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.TypingPosition, currentPos.TypingPosition,
			),
			ReceiptPosition: rp.streams.ReceiptStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.ReceiptPosition, currentPos.ReceiptPosition,
			),
			InvitePosition: rp.streams.InviteStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.InvitePosition, currentPos.InvitePosition,
			),
			SendToDevicePosition: rp.streams.SendToDeviceStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.SendToDevicePosition, currentPos.SendToDevicePosition,
			),
			AccountDataPosition: rp.streams.AccountDataStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.AccountDataPosition, currentPos.AccountDataPosition,
			),
			DeviceListPosition: rp.streams.DeviceListStreamProvider.IncrementalSync(
				syncReq.Context, syncReq,
				syncReq.Since.DeviceListPosition, currentPos.DeviceListPosition,
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
	syncReq, err := newSyncRequest(req, *device, rp.db)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("newSyncRequest failed")
		return jsonerror.InternalServerError()
	}
	rp.streams.PDUStreamProvider.IncrementalSync(req.Context(), syncReq, fromToken.PDUPosition, toToken.PDUPosition)
	_, _, err = internal.DeviceListCatchup(
		req.Context(), rp.keyAPI, rp.rsAPI, syncReq.Device.UserID,
		syncReq.Response, fromToken.DeviceListPosition, toToken.DeviceListPosition,
	)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("Failed to DeviceListCatchup info")
		return jsonerror.InternalServerError()
	}
	return util.JSONResponse{
		Code: 200,
		JSON: struct {
			Changed []string `json:"changed"`
			Left    []string `json:"left"`
		}{
			Changed: syncReq.Response.DeviceLists.Changed,
			Left:    syncReq.Response.DeviceLists.Left,
		},
	}
}

// shouldReturnImmediately returns whether the /sync request is an initial sync,
// or timeout=0, or full_state=true, in any of the cases the request should
// return immediately.
func (rp *RequestPool) shouldReturnImmediately(syncReq *types.SyncRequest, currentPos types.StreamingToken) bool {
	if currentPos.IsAfter(syncReq.Since) || syncReq.Timeout == 0 || syncReq.WantFullState {
		return true
	}
	return false
}
