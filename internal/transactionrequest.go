// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/federationapi/producers"
	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/dendrite/roomserver/api"
	syncTypes "github.com/matrix-org/dendrite/syncapi/types"
	userAPI "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

var (
	PDUCountTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "dendrite",
			Subsystem: "federationapi",
			Name:      "recv_pdus",
			Help:      "Number of incoming PDUs from remote servers with labels for success",
		},
		[]string{"status"}, // 'success' or 'total'
	)
	EDUCountTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "dendrite",
			Subsystem: "federationapi",
			Name:      "recv_edus",
			Help:      "Number of incoming EDUs from remote servers",
		},
	)
)

type TxnReq struct {
	gomatrixserverlib.Transaction
	rsAPI                  api.FederationRoomserverAPI
	userAPI                userAPI.FederationUserAPI
	ourServerName          spec.ServerName
	keys                   gomatrixserverlib.JSONVerifier
	roomsMu                *MutexByRoom
	producer               *producers.SyncAPIProducer
	inboundPresenceEnabled bool
}

func NewTxnReq(
	rsAPI api.FederationRoomserverAPI,
	userAPI userAPI.FederationUserAPI,
	ourServerName spec.ServerName,
	keys gomatrixserverlib.JSONVerifier,
	roomsMu *MutexByRoom,
	producer *producers.SyncAPIProducer,
	inboundPresenceEnabled bool,
	pdus []json.RawMessage,
	edus []gomatrixserverlib.EDU,
	origin spec.ServerName,
	transactionID gomatrixserverlib.TransactionID,
	destination spec.ServerName,
) TxnReq {
	t := TxnReq{
		rsAPI:                  rsAPI,
		userAPI:                userAPI,
		ourServerName:          ourServerName,
		keys:                   keys,
		roomsMu:                roomsMu,
		producer:               producer,
		inboundPresenceEnabled: inboundPresenceEnabled,
	}

	t.PDUs = pdus
	t.EDUs = edus
	t.Origin = origin
	t.TransactionID = transactionID
	t.Destination = destination

	return t
}

func (t *TxnReq) ProcessTransaction(ctx context.Context) (*fclient.RespSend, *util.JSONResponse) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if t.producer != nil {
			t.processEDUs(ctx)
		}
	}()

	results := make(map[string]fclient.PDUResult)
	roomVersions := make(map[string]gomatrixserverlib.RoomVersion)
	getRoomVersion := func(roomID string) gomatrixserverlib.RoomVersion {
		if v, ok := roomVersions[roomID]; ok {
			return v
		}
		verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
		verRes := api.QueryRoomVersionForRoomResponse{}
		if err := t.rsAPI.QueryRoomVersionForRoom(ctx, &verReq, &verRes); err != nil {
			util.GetLogger(ctx).WithError(err).Debug("Transaction: Failed to query room version for room", verReq.RoomID)
			return ""
		}
		roomVersions[roomID] = verRes.RoomVersion
		return verRes.RoomVersion
	}

	for _, pdu := range t.PDUs {
		PDUCountTotal.WithLabelValues("total").Inc()
		var header struct {
			RoomID string `json:"room_id"`
		}
		if err := json.Unmarshal(pdu, &header); err != nil {
			util.GetLogger(ctx).WithError(err).Debug("Transaction: Failed to extract room ID from event")
			// We don't know the event ID at this point so we can't return the
			// failure in the PDU results
			continue
		}
		roomVersion := getRoomVersion(header.RoomID)
		verImpl, err := gomatrixserverlib.GetRoomVersion(roomVersion)
		if err != nil {
			continue
		}
		event, err := verImpl.NewEventFromUntrustedJSON(pdu)
		if err != nil {
			if _, ok := err.(gomatrixserverlib.BadJSONError); ok {
				// Room version 6 states that homeservers should strictly enforce canonical JSON
				// on PDUs.
				//
				// This enforces that the entire transaction is rejected if a single bad PDU is
				// sent. It is unclear if this is the correct behaviour or not.
				//
				// See https://github.com/matrix-org/synapse/issues/7543
				return nil, &util.JSONResponse{
					Code: 400,
					JSON: jsonerror.BadJSON("PDU contains bad JSON"),
				}
			}
			util.GetLogger(ctx).WithError(err).Debugf("Transaction: Failed to parse event JSON of event %s", string(pdu))
			continue
		}
		if event.Type() == spec.MRoomCreate && event.StateKeyEquals("") {
			continue
		}
		if api.IsServerBannedFromRoom(ctx, t.rsAPI, event.RoomID(), t.Origin) {
			results[event.EventID()] = fclient.PDUResult{
				Error: "Forbidden by server ACLs",
			}
			continue
		}
		if err = event.VerifyEventSignatures(ctx, t.keys); err != nil {
			util.GetLogger(ctx).WithError(err).Debugf("Transaction: Couldn't validate signature of event %q", event.EventID())
			results[event.EventID()] = fclient.PDUResult{
				Error: err.Error(),
			}
			continue
		}

		// pass the event to the roomserver which will do auth checks
		// If the event fail auth checks, gmsl.NotAllowed error will be returned which we be silently
		// discarded by the caller of this function
		if err = api.SendEvents(
			ctx,
			t.rsAPI,
			api.KindNew,
			[]*gomatrixserverlib.HeaderedEvent{
				event.Headered(roomVersion),
			},
			t.Destination,
			t.Origin,
			api.DoNotSendToOtherServers,
			nil,
			true,
		); err != nil {
			util.GetLogger(ctx).WithError(err).Errorf("Transaction: Couldn't submit event %q to input queue: %s", event.EventID(), err)
			results[event.EventID()] = fclient.PDUResult{
				Error: err.Error(),
			}
			continue
		}

		results[event.EventID()] = fclient.PDUResult{}
		PDUCountTotal.WithLabelValues("success").Inc()
	}

	wg.Wait()
	return &fclient.RespSend{PDUs: results}, nil
}

// nolint:gocyclo
func (t *TxnReq) processEDUs(ctx context.Context) {
	for _, e := range t.EDUs {
		EDUCountTotal.Inc()
		switch e.Type {
		case spec.MTyping:
			// https://matrix.org/docs/spec/server_server/latest#typing-notifications
			var typingPayload struct {
				RoomID string `json:"room_id"`
				UserID string `json:"user_id"`
				Typing bool   `json:"typing"`
			}
			if err := json.Unmarshal(e.Content, &typingPayload); err != nil {
				util.GetLogger(ctx).WithError(err).Debug("Failed to unmarshal typing event")
				continue
			}
			if _, serverName, err := gomatrixserverlib.SplitID('@', typingPayload.UserID); err != nil {
				continue
			} else if serverName == t.ourServerName {
				continue
			} else if serverName != t.Origin {
				continue
			}
			if err := t.producer.SendTyping(ctx, typingPayload.UserID, typingPayload.RoomID, typingPayload.Typing, 30*1000); err != nil {
				util.GetLogger(ctx).WithError(err).Error("Failed to send typing event to JetStream")
			}
		case spec.MDirectToDevice:
			// https://matrix.org/docs/spec/server_server/r0.1.3#m-direct-to-device-schema
			var directPayload gomatrixserverlib.ToDeviceMessage
			if err := json.Unmarshal(e.Content, &directPayload); err != nil {
				util.GetLogger(ctx).WithError(err).Debug("Failed to unmarshal send-to-device events")
				continue
			}
			if _, serverName, err := gomatrixserverlib.SplitID('@', directPayload.Sender); err != nil {
				continue
			} else if serverName == t.ourServerName {
				continue
			} else if serverName != t.Origin {
				continue
			}
			for userID, byUser := range directPayload.Messages {
				for deviceID, message := range byUser {
					// TODO: check that the user and the device actually exist here
					if err := t.producer.SendToDevice(ctx, directPayload.Sender, userID, deviceID, directPayload.Type, message); err != nil {
						sentry.CaptureException(err)
						util.GetLogger(ctx).WithError(err).WithFields(logrus.Fields{
							"sender":    directPayload.Sender,
							"user_id":   userID,
							"device_id": deviceID,
						}).Error("Failed to send send-to-device event to JetStream")
					}
				}
			}
		case spec.MDeviceListUpdate:
			if err := t.producer.SendDeviceListUpdate(ctx, e.Content, t.Origin); err != nil {
				sentry.CaptureException(err)
				util.GetLogger(ctx).WithError(err).Error("failed to InputDeviceListUpdate")
			}
		case spec.MReceipt:
			// https://matrix.org/docs/spec/server_server/r0.1.4#receipts
			payload := map[string]types.FederationReceiptMRead{}

			if err := json.Unmarshal(e.Content, &payload); err != nil {
				util.GetLogger(ctx).WithError(err).Debug("Failed to unmarshal receipt event")
				continue
			}

			for roomID, receipt := range payload {
				for userID, mread := range receipt.User {
					_, domain, err := gomatrixserverlib.SplitID('@', userID)
					if err != nil {
						util.GetLogger(ctx).WithError(err).Debug("Failed to split domain from receipt event sender")
						continue
					}
					if t.Origin != domain {
						util.GetLogger(ctx).Debugf("Dropping receipt event where sender domain (%q) doesn't match origin (%q)", domain, t.Origin)
						continue
					}
					if err := t.processReceiptEvent(ctx, userID, roomID, "m.read", mread.Data.TS, mread.EventIDs); err != nil {
						util.GetLogger(ctx).WithError(err).WithFields(logrus.Fields{
							"sender":  t.Origin,
							"user_id": userID,
							"room_id": roomID,
							"events":  mread.EventIDs,
						}).Error("Failed to send receipt event to JetStream")
						continue
					}
				}
			}
		case types.MSigningKeyUpdate:
			if err := t.producer.SendSigningKeyUpdate(ctx, e.Content, t.Origin); err != nil {
				sentry.CaptureException(err)
				logrus.WithError(err).Errorf("Failed to process signing key update")
			}
		case spec.MPresence:
			if t.inboundPresenceEnabled {
				if err := t.processPresence(ctx, e); err != nil {
					logrus.WithError(err).Errorf("Failed to process presence update")
				}
			}
		default:
			util.GetLogger(ctx).WithField("type", e.Type).Debug("Unhandled EDU")
		}
	}
}

// processPresence handles m.receipt events
func (t *TxnReq) processPresence(ctx context.Context, e gomatrixserverlib.EDU) error {
	payload := types.Presence{}
	if err := json.Unmarshal(e.Content, &payload); err != nil {
		return err
	}
	for _, content := range payload.Push {
		if _, serverName, err := gomatrixserverlib.SplitID('@', content.UserID); err != nil {
			continue
		} else if serverName == t.ourServerName {
			continue
		} else if serverName != t.Origin {
			continue
		}
		presence, ok := syncTypes.PresenceFromString(content.Presence)
		if !ok {
			continue
		}
		if err := t.producer.SendPresence(ctx, content.UserID, presence, content.StatusMsg, content.LastActiveAgo); err != nil {
			return err
		}
	}
	return nil
}

// processReceiptEvent sends receipt events to JetStream
func (t *TxnReq) processReceiptEvent(ctx context.Context,
	userID, roomID, receiptType string,
	timestamp spec.Timestamp,
	eventIDs []string,
) error {
	if _, serverName, err := gomatrixserverlib.SplitID('@', userID); err != nil {
		return nil
	} else if serverName == t.ourServerName {
		return nil
	} else if serverName != t.Origin {
		return nil
	}
	// store every event
	for _, eventID := range eventIDs {
		if err := t.producer.SendReceipt(ctx, userID, roomID, eventID, receiptType, timestamp); err != nil {
			return fmt.Errorf("unable to set receipt event: %w", err)
		}
	}

	return nil
}
