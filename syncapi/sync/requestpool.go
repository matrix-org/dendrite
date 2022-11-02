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
	"database/sql"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/internal"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/streams"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// RequestPool manages HTTP long-poll connections for /sync
type RequestPool struct {
	db       storage.Database
	cfg      *config.SyncAPI
	userAPI  userapi.SyncUserAPI
	keyAPI   keyapi.SyncKeyAPI
	rsAPI    roomserverAPI.SyncRoomserverAPI
	lastseen *sync.Map
	presence *sync.Map
	streams  *streams.Streams
	Notifier *notifier.Notifier
	producer PresencePublisher
	consumer PresenceConsumer
}

type PresencePublisher interface {
	SendPresence(userID string, presence types.Presence, statusMsg *string) error
}

type PresenceConsumer interface {
	EmitPresence(ctx context.Context, userID string, presence types.Presence, statusMsg *string, ts gomatrixserverlib.Timestamp, fromSync bool)
}

// NewRequestPool makes a new RequestPool
func NewRequestPool(
	db storage.Database, cfg *config.SyncAPI,
	userAPI userapi.SyncUserAPI, keyAPI keyapi.SyncKeyAPI,
	rsAPI roomserverAPI.SyncRoomserverAPI,
	streams *streams.Streams, notifier *notifier.Notifier,
	producer PresencePublisher, consumer PresenceConsumer, enableMetrics bool,
) *RequestPool {
	if enableMetrics {
		prometheus.MustRegister(
			activeSyncRequests, waitingSyncRequests,
		)
	}
	rp := &RequestPool{
		db:       db,
		cfg:      cfg,
		userAPI:  userAPI,
		keyAPI:   keyAPI,
		rsAPI:    rsAPI,
		lastseen: &sync.Map{},
		presence: &sync.Map{},
		streams:  streams,
		Notifier: notifier,
		producer: producer,
		consumer: consumer,
	}
	go rp.cleanLastSeen()
	go rp.cleanPresence(db, time.Minute*5)
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

func (rp *RequestPool) cleanPresence(db storage.Presence, cleanupTime time.Duration) {
	if !rp.cfg.Matrix.Presence.EnableOutbound {
		return
	}
	for {
		rp.presence.Range(func(key interface{}, v interface{}) bool {
			p := v.(types.PresenceInternal)
			if time.Since(p.LastActiveTS.Time()) > cleanupTime {
				rp.updatePresence(db, types.PresenceUnavailable.String(), p.UserID)
				rp.presence.Delete(key)
			}
			return true
		})
		time.Sleep(cleanupTime)
	}
}

// updatePresence sends presence updates to the SyncAPI and FederationAPI
func (rp *RequestPool) updatePresence(db storage.Presence, presence string, userID string) {
	if !rp.cfg.Matrix.Presence.EnableOutbound {
		return
	}
	if presence == "" {
		presence = types.PresenceOnline.String()
	}

	presenceID, ok := types.PresenceFromString(presence)
	if !ok { // this should almost never happen
		return
	}

	newPresence := types.PresenceInternal{
		Presence:     presenceID,
		UserID:       userID,
		LastActiveTS: gomatrixserverlib.AsTimestamp(time.Now()),
	}

	// ensure we also send the current status_msg to federated servers and not nil
	dbPresence, err := db.GetPresence(context.Background(), userID)
	if err != nil && err != sql.ErrNoRows {
		return
	}
	if dbPresence != nil {
		newPresence.ClientFields = dbPresence.ClientFields
	}
	newPresence.ClientFields.Presence = presenceID.String()

	defer rp.presence.Store(userID, newPresence)
	// avoid spamming presence updates when syncing
	existingPresence, ok := rp.presence.LoadOrStore(userID, newPresence)
	if ok {
		p := existingPresence.(types.PresenceInternal)
		if p.ClientFields.Presence == newPresence.ClientFields.Presence {
			return
		}
	}

	if err := rp.producer.SendPresence(userID, presenceID, newPresence.ClientFields.StatusMsg); err != nil {
		logrus.WithError(err).Error("Unable to publish presence message from sync")
		return
	}

	// now synchronously update our view of the world. It's critical we do this before calculating
	// the /sync response else we may not return presence: online immediately.
	rp.consumer.EmitPresence(
		context.Background(), userID, presenceID, newPresence.ClientFields.StatusMsg,
		gomatrixserverlib.AsTimestamp(time.Now()), true,
	)
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
		UserAgent:  req.UserAgent(),
	}
	lsres := &userapi.PerformLastSeenUpdateResponse{}
	go rp.userAPI.PerformLastSeenUpdate(req.Context(), lsreq, lsres) // nolint:errcheck

	rp.lastseen.Store(device.UserID+device.ID, time.Now())
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
		if err == types.ErrMalformedSyncToken {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidArgumentValue(err.Error()),
			}
		}
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}

	activeSyncRequests.Inc()
	defer activeSyncRequests.Dec()

	rp.updateLastSeen(req, device)
	rp.updatePresence(rp.db, req.FormValue("set_presence"), device.UserID)

	waitingSyncRequests.Inc()
	defer waitingSyncRequests.Dec()

	// Clean up old send-to-device messages from before this stream position.
	// This is needed to avoid sending the same message multiple times
	if err = rp.db.CleanSendToDeviceUpdates(syncReq.Context, syncReq.Device.UserID, syncReq.Device.ID, syncReq.Since.SendToDevicePosition); err != nil {
		syncReq.Log.WithError(err).Error("p.DB.CleanSendToDeviceUpdates failed")
	}

	// loop until we get some data
	for {
		startTime := time.Now()
		currentPos := rp.Notifier.CurrentPosition()

		// if the since token matches the current positions, wait via the notifier
		if !rp.shouldReturnImmediately(syncReq, currentPos) {
			timer := time.NewTimer(syncReq.Timeout) // case of timeout=0 is handled above
			defer timer.Stop()

			userStreamListener := rp.Notifier.GetListener(*syncReq)
			defer userStreamListener.Close()

			giveup := func() util.JSONResponse {
				syncReq.Log.Debugln("Responding to sync since client gave up or timeout was reached")
				syncReq.Response.NextBatch = syncReq.Since
				// We should always try to include OTKs in sync responses, otherwise clients might upload keys
				// even if that's not required. See also:
				// https://github.com/matrix-org/synapse/blob/29f06704b8871a44926f7c99e73cf4a978fb8e81/synapse/rest/client/sync.py#L276-L281
				// Only try to get OTKs if the context isn't already done.
				if syncReq.Context.Err() == nil {
					err = internal.DeviceOTKCounts(syncReq.Context, rp.keyAPI, syncReq.Device.UserID, syncReq.Device.ID, syncReq.Response)
					if err != nil && err != context.Canceled {
						syncReq.Log.WithError(err).Warn("failed to get OTK counts")
					}
				}
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
				currentPos.ApplyUpdates(userStreamListener.GetSyncPosition())
				syncReq.Log.WithField("currentPos", currentPos).Debugln("Responding to sync after wake-up")
			}
		} else {
			syncReq.Log.WithField("currentPos", currentPos).Debugln("Responding to sync immediately")
		}

		withTransaction := func(from types.StreamPosition, f func(snapshot storage.DatabaseTransaction) types.StreamPosition) types.StreamPosition {
			var succeeded bool
			snapshot, err := rp.db.NewDatabaseSnapshot(req.Context())
			if err != nil {
				logrus.WithError(err).Error("Failed to acquire database snapshot for sync request")
				return from
			}
			defer func() {
				succeeded = err == nil
				sqlutil.EndTransactionWithCheck(snapshot, &succeeded, &err)
			}()
			return f(snapshot)
		}

		if syncReq.Since.IsEmpty() {
			// Complete sync
			syncReq.Response.NextBatch = types.StreamingToken{
				// Get the current DeviceListPosition first, as the currentPosition
				// might advance while processing other streams, resulting in flakey
				// tests.
				DeviceListPosition: withTransaction(
					syncReq.Since.DeviceListPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.DeviceListStreamProvider.CompleteSync(
							syncReq.Context, txn, syncReq,
						)
					},
				),
				PDUPosition: withTransaction(
					syncReq.Since.PDUPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.PDUStreamProvider.CompleteSync(
							syncReq.Context, txn, syncReq,
						)
					},
				),
				TypingPosition: withTransaction(
					syncReq.Since.TypingPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.TypingStreamProvider.CompleteSync(
							syncReq.Context, txn, syncReq,
						)
					},
				),
				ReceiptPosition: withTransaction(
					syncReq.Since.ReceiptPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.ReceiptStreamProvider.CompleteSync(
							syncReq.Context, txn, syncReq,
						)
					},
				),
				InvitePosition: withTransaction(
					syncReq.Since.InvitePosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.InviteStreamProvider.CompleteSync(
							syncReq.Context, txn, syncReq,
						)
					},
				),
				SendToDevicePosition: withTransaction(
					syncReq.Since.SendToDevicePosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.SendToDeviceStreamProvider.CompleteSync(
							syncReq.Context, txn, syncReq,
						)
					},
				),
				AccountDataPosition: withTransaction(
					syncReq.Since.AccountDataPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.AccountDataStreamProvider.CompleteSync(
							syncReq.Context, txn, syncReq,
						)
					},
				),
				NotificationDataPosition: withTransaction(
					syncReq.Since.NotificationDataPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.NotificationDataStreamProvider.CompleteSync(
							syncReq.Context, txn, syncReq,
						)
					},
				),
				PresencePosition: withTransaction(
					syncReq.Since.PresencePosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.PresenceStreamProvider.CompleteSync(
							syncReq.Context, txn, syncReq,
						)
					},
				),
			}
		} else {
			// Incremental sync
			syncReq.Response.NextBatch = types.StreamingToken{
				PDUPosition: withTransaction(
					syncReq.Since.PDUPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.PDUStreamProvider.IncrementalSync(
							syncReq.Context, txn, syncReq,
							syncReq.Since.PDUPosition, rp.Notifier.CurrentPosition().PDUPosition,
						)
					},
				),
				TypingPosition: withTransaction(
					syncReq.Since.TypingPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.TypingStreamProvider.IncrementalSync(
							syncReq.Context, txn, syncReq,
							syncReq.Since.TypingPosition, rp.Notifier.CurrentPosition().TypingPosition,
						)
					},
				),
				ReceiptPosition: withTransaction(
					syncReq.Since.ReceiptPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.ReceiptStreamProvider.IncrementalSync(
							syncReq.Context, txn, syncReq,
							syncReq.Since.ReceiptPosition, rp.Notifier.CurrentPosition().ReceiptPosition,
						)
					},
				),
				InvitePosition: withTransaction(
					syncReq.Since.InvitePosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.InviteStreamProvider.IncrementalSync(
							syncReq.Context, txn, syncReq,
							syncReq.Since.InvitePosition, rp.Notifier.CurrentPosition().InvitePosition,
						)
					},
				),
				SendToDevicePosition: withTransaction(
					syncReq.Since.SendToDevicePosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.SendToDeviceStreamProvider.IncrementalSync(
							syncReq.Context, txn, syncReq,
							syncReq.Since.SendToDevicePosition, rp.Notifier.CurrentPosition().SendToDevicePosition,
						)
					},
				),
				AccountDataPosition: withTransaction(
					syncReq.Since.AccountDataPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.AccountDataStreamProvider.IncrementalSync(
							syncReq.Context, txn, syncReq,
							syncReq.Since.AccountDataPosition, rp.Notifier.CurrentPosition().AccountDataPosition,
						)
					},
				),
				NotificationDataPosition: withTransaction(
					syncReq.Since.NotificationDataPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.NotificationDataStreamProvider.IncrementalSync(
							syncReq.Context, txn, syncReq,
							syncReq.Since.NotificationDataPosition, rp.Notifier.CurrentPosition().NotificationDataPosition,
						)
					},
				),
				DeviceListPosition: withTransaction(
					syncReq.Since.DeviceListPosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.DeviceListStreamProvider.IncrementalSync(
							syncReq.Context, txn, syncReq,
							syncReq.Since.DeviceListPosition, rp.Notifier.CurrentPosition().DeviceListPosition,
						)
					},
				),
				PresencePosition: withTransaction(
					syncReq.Since.PresencePosition,
					func(txn storage.DatabaseTransaction) types.StreamPosition {
						return rp.streams.PresenceStreamProvider.IncrementalSync(
							syncReq.Context, txn, syncReq,
							syncReq.Since.PresencePosition, rp.Notifier.CurrentPosition().PresencePosition,
						)
					},
				),
			}
			// it's possible for there to be no updates for this user even though since < current pos,
			// e.g busy servers with a quiet user. In this scenario, we don't want to return a no-op
			// response immediately, so let's try this again but pretend they bumped their since token.
			// If the incremental sync was processed very quickly then we expect the next loop to block
			// with a notifier, but if things are slow it's entirely possible that currentPos is no
			// longer the current position so we will hit this code path again. We need to do this and
			// not return a no-op response because:
			// - It's an inefficient use of bandwidth.
			// - Some sytests which test 'waking up' sync rely on some sync requests to block, which
			//   they weren't always doing, resulting in flakey tests.
			if !syncReq.Response.HasUpdates() {
				syncReq.Since = currentPos
				// do not loop again if the ?timeout= is 0 as that means "return immediately"
				if syncReq.Timeout > 0 {
					syncReq.Timeout = syncReq.Timeout - time.Since(startTime)
					if syncReq.Timeout < 0 {
						syncReq.Timeout = 0
					}
					continue
				}
			}
		}

		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: syncReq.Response,
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
	syncReq, err := newSyncRequest(req, *device, rp.db)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("newSyncRequest failed")
		return jsonerror.InternalServerError()
	}
	snapshot, err := rp.db.NewDatabaseSnapshot(req.Context())
	if err != nil {
		logrus.WithError(err).Error("Failed to acquire database snapshot for key change")
		return jsonerror.InternalServerError()
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(snapshot, &succeeded, &err)
	rp.streams.PDUStreamProvider.IncrementalSync(req.Context(), snapshot, syncReq, fromToken.PDUPosition, toToken.PDUPosition)
	_, _, err = internal.DeviceListCatchup(
		req.Context(), snapshot, rp.keyAPI, rp.rsAPI, syncReq.Device.UserID,
		syncReq.Response, fromToken.DeviceListPosition, toToken.DeviceListPosition,
	)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("Failed to DeviceListCatchup info")
		return jsonerror.InternalServerError()
	}
	succeeded = true
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
