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

package routing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	eduserverAPI "github.com/matrix-org/dendrite/eduserver/api"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// Send implements /_matrix/federation/v1/send/{txnID}
func Send(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	txnID gomatrixserverlib.TransactionID,
	cfg *config.FederationAPI,
	rsAPI api.RoomserverInternalAPI,
	eduAPI eduserverAPI.EDUServerInputAPI,
	keyAPI keyapi.KeyInternalAPI,
	keys gomatrixserverlib.JSONVerifier,
	federation *gomatrixserverlib.FederationClient,
) util.JSONResponse {
	t := txnReq{
		rsAPI:      rsAPI,
		eduAPI:     eduAPI,
		keys:       keys,
		federation: federation,
		haveEvents: make(map[string]*gomatrixserverlib.HeaderedEvent),
		newEvents:  make(map[string]bool),
		keyAPI:     keyAPI,
	}

	var txnEvents struct {
		PDUs []json.RawMessage       `json:"pdus"`
		EDUs []gomatrixserverlib.EDU `json:"edus"`
	}

	if err := json.Unmarshal(request.Content(), &txnEvents); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}
	// Transactions are limited in size; they can have at most 50 PDUs and 100 EDUs.
	// https://matrix.org/docs/spec/server_server/latest#transactions
	if len(txnEvents.PDUs) > 50 || len(txnEvents.EDUs) > 100 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("max 50 pdus / 100 edus"),
		}
	}

	// TODO: Really we should have a function to convert FederationRequest to txnReq
	t.PDUs = txnEvents.PDUs
	t.EDUs = txnEvents.EDUs
	t.Origin = request.Origin()
	t.TransactionID = txnID
	t.Destination = cfg.Matrix.ServerName

	util.GetLogger(httpReq.Context()).Infof("Received transaction %q from %q containing %d PDUs, %d EDUs", txnID, request.Origin(), len(t.PDUs), len(t.EDUs))

	resp, jsonErr := t.processTransaction(context.Background())
	if jsonErr != nil {
		util.GetLogger(httpReq.Context()).WithField("jsonErr", jsonErr).Error("t.processTransaction failed")
		return *jsonErr
	}

	// https://matrix.org/docs/spec/server_server/r0.1.3#put-matrix-federation-v1-send-txnid
	// Status code 200:
	// The result of processing the transaction. The server is to use this response
	// even in the event of one or more PDUs failing to be processed.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}

type txnReq struct {
	gomatrixserverlib.Transaction
	rsAPI      api.RoomserverInternalAPI
	eduAPI     eduserverAPI.EDUServerInputAPI
	keyAPI     keyapi.KeyInternalAPI
	keys       gomatrixserverlib.JSONVerifier
	federation txnFederationClient
	// local cache of events for auth checks, etc - this may include events
	// which the roomserver is unaware of.
	haveEvents map[string]*gomatrixserverlib.HeaderedEvent
	// new events which the roomserver does not know about
	newEvents      map[string]bool
	newEventsMutex sync.RWMutex
}

// A subset of FederationClient functionality that txn requires. Useful for testing.
type txnFederationClient interface {
	LookupState(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string, roomVersion gomatrixserverlib.RoomVersion) (
		res gomatrixserverlib.RespState, err error,
	)
	LookupStateIDs(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string) (res gomatrixserverlib.RespStateIDs, err error)
	GetEvent(ctx context.Context, s gomatrixserverlib.ServerName, eventID string) (res gomatrixserverlib.Transaction, err error)
	LookupMissingEvents(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, missing gomatrixserverlib.MissingEvents,
		roomVersion gomatrixserverlib.RoomVersion) (res gomatrixserverlib.RespMissingEvents, err error)
}

func (t *txnReq) processTransaction(ctx context.Context) (*gomatrixserverlib.RespSend, *util.JSONResponse) {
	results := make(map[string]gomatrixserverlib.PDUResult)

	pdus := []*gomatrixserverlib.HeaderedEvent{}
	for _, pdu := range t.PDUs {
		var header struct {
			RoomID string `json:"room_id"`
		}
		if err := json.Unmarshal(pdu, &header); err != nil {
			util.GetLogger(ctx).WithError(err).Warn("Transaction: Failed to extract room ID from event")
			// We don't know the event ID at this point so we can't return the
			// failure in the PDU results
			continue
		}
		verReq := api.QueryRoomVersionForRoomRequest{RoomID: header.RoomID}
		verRes := api.QueryRoomVersionForRoomResponse{}
		if err := t.rsAPI.QueryRoomVersionForRoom(ctx, &verReq, &verRes); err != nil {
			util.GetLogger(ctx).WithError(err).Warn("Transaction: Failed to query room version for room", verReq.RoomID)
			// We don't know the event ID at this point so we can't return the
			// failure in the PDU results
			continue
		}
		event, err := gomatrixserverlib.NewEventFromUntrustedJSON(pdu, verRes.RoomVersion)
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
			util.GetLogger(ctx).WithError(err).Warnf("Transaction: Failed to parse event JSON of event %s", string(pdu))
			continue
		}
		if api.IsServerBannedFromRoom(ctx, t.rsAPI, event.RoomID(), t.Origin) {
			results[event.EventID()] = gomatrixserverlib.PDUResult{
				Error: "Forbidden by server ACLs",
			}
			continue
		}
		if err = gomatrixserverlib.VerifyAllEventSignatures(ctx, []*gomatrixserverlib.Event{event}, t.keys); err != nil {
			util.GetLogger(ctx).WithError(err).Warnf("Transaction: Couldn't validate signature of event %q", event.EventID())
			results[event.EventID()] = gomatrixserverlib.PDUResult{
				Error: err.Error(),
			}
			continue
		}
		pdus = append(pdus, event.Headered(verRes.RoomVersion))
	}

	// Process the events.
	for _, e := range pdus {
		if err := t.processEvent(ctx, e.Unwrap()); err != nil {
			// If the error is due to the event itself being bad then we skip
			// it and move onto the next event. We report an error so that the
			// sender knows that we have skipped processing it.
			//
			// However if the event is due to a temporary failure in our server
			// such as a database being unavailable then we should bail, and
			// hope that the sender will retry when we are feeling better.
			//
			// It is uncertain what we should do if an event fails because
			// we failed to fetch more information from the sending server.
			// For example if a request to /state fails.
			// If we skip the event then we risk missing the event until we
			// receive another event referencing it.
			// If we bail and stop processing then we risk wedging incoming
			// transactions from that server forever.
			if isProcessingErrorFatal(err) {
				// Any other error should be the result of a temporary error in
				// our server so we should bail processing the transaction entirely.
				util.GetLogger(ctx).Warnf("Processing %s failed fatally: %s", e.EventID(), err)
				jsonErr := util.ErrorResponse(err)
				return nil, &jsonErr
			} else {
				// Auth errors mean the event is 'rejected' which have to be silent to appease sytest
				errMsg := ""
				_, rejected := err.(*gomatrixserverlib.NotAllowed)
				if !rejected {
					errMsg = err.Error()
				}
				util.GetLogger(ctx).WithError(err).WithField("event_id", e.EventID()).WithField("rejected", rejected).Warn(
					"Failed to process incoming federation event, skipping",
				)
				results[e.EventID()] = gomatrixserverlib.PDUResult{
					Error: errMsg,
				}
			}
		} else {
			results[e.EventID()] = gomatrixserverlib.PDUResult{}
		}
	}

	t.processEDUs(ctx)
	if c := len(results); c > 0 {
		util.GetLogger(ctx).Infof("Processed %d PDUs from transaction %q", c, t.TransactionID)
	}
	return &gomatrixserverlib.RespSend{PDUs: results}, nil
}

// isProcessingErrorFatal returns true if the error is really bad and
// we should stop processing the transaction, and returns false if it
// is just some less serious error about a specific event.
func isProcessingErrorFatal(err error) bool {
	switch err {
	case sql.ErrConnDone:
	case sql.ErrTxDone:
		return true
	}
	return false
}

type roomNotFoundError struct {
	roomID string
}
type verifySigError struct {
	eventID string
	err     error
}
type missingPrevEventsError struct {
	eventID string
	err     error
}

func (e roomNotFoundError) Error() string { return fmt.Sprintf("room %q not found", e.roomID) }
func (e verifySigError) Error() string {
	return fmt.Sprintf("unable to verify signature of event %q: %s", e.eventID, e.err)
}
func (e missingPrevEventsError) Error() string {
	return fmt.Sprintf("unable to get prev_events for event %q: %s", e.eventID, e.err)
}

func (t *txnReq) haveEventIDs() map[string]bool {
	t.newEventsMutex.RLock()
	defer t.newEventsMutex.RUnlock()
	result := make(map[string]bool, len(t.haveEvents))
	for eventID := range t.haveEvents {
		if t.newEvents[eventID] {
			continue
		}
		result[eventID] = true
	}
	return result
}

// nolint:gocyclo
func (t *txnReq) processEDUs(ctx context.Context) {
	for _, e := range t.EDUs {
		switch e.Type {
		case gomatrixserverlib.MTyping:
			// https://matrix.org/docs/spec/server_server/latest#typing-notifications
			var typingPayload struct {
				RoomID string `json:"room_id"`
				UserID string `json:"user_id"`
				Typing bool   `json:"typing"`
			}
			if err := json.Unmarshal(e.Content, &typingPayload); err != nil {
				util.GetLogger(ctx).WithError(err).Error("Failed to unmarshal typing event")
				continue
			}
			_, domain, err := gomatrixserverlib.SplitID('@', typingPayload.UserID)
			if err != nil {
				util.GetLogger(ctx).WithError(err).Error("Failed to split domain from typing event sender")
				continue
			}
			if domain != t.Origin {
				util.GetLogger(ctx).Warnf("Dropping typing event where sender domain (%q) doesn't match origin (%q)", domain, t.Origin)
				continue
			}
			if err := eduserverAPI.SendTyping(ctx, t.eduAPI, typingPayload.UserID, typingPayload.RoomID, typingPayload.Typing, 30*1000); err != nil {
				util.GetLogger(ctx).WithError(err).Error("Failed to send typing event to edu server")
			}
		case gomatrixserverlib.MDirectToDevice:
			// https://matrix.org/docs/spec/server_server/r0.1.3#m-direct-to-device-schema
			var directPayload gomatrixserverlib.ToDeviceMessage
			if err := json.Unmarshal(e.Content, &directPayload); err != nil {
				util.GetLogger(ctx).WithError(err).Error("Failed to unmarshal send-to-device events")
				continue
			}
			for userID, byUser := range directPayload.Messages {
				for deviceID, message := range byUser {
					// TODO: check that the user and the device actually exist here
					if err := eduserverAPI.SendToDevice(ctx, t.eduAPI, directPayload.Sender, userID, deviceID, directPayload.Type, message); err != nil {
						util.GetLogger(ctx).WithError(err).WithFields(logrus.Fields{
							"sender":    directPayload.Sender,
							"user_id":   userID,
							"device_id": deviceID,
						}).Error("Failed to send send-to-device event to edu server")
					}
				}
			}
		case gomatrixserverlib.MDeviceListUpdate:
			t.processDeviceListUpdate(ctx, e)
		case gomatrixserverlib.MReceipt:
			// https://matrix.org/docs/spec/server_server/r0.1.4#receipts
			payload := map[string]eduserverAPI.FederationReceiptMRead{}

			if err := json.Unmarshal(e.Content, &payload); err != nil {
				util.GetLogger(ctx).WithError(err).Error("Failed to unmarshal receipt event")
				continue
			}

			for roomID, receipt := range payload {
				for userID, mread := range receipt.User {
					_, domain, err := gomatrixserverlib.SplitID('@', userID)
					if err != nil {
						util.GetLogger(ctx).WithError(err).Error("Failed to split domain from receipt event sender")
						continue
					}
					if t.Origin != domain {
						util.GetLogger(ctx).Warnf("Dropping receipt event where sender domain (%q) doesn't match origin (%q)", domain, t.Origin)
						continue
					}
					if err := t.processReceiptEvent(ctx, userID, roomID, "m.read", mread.Data.TS, mread.EventIDs); err != nil {
						util.GetLogger(ctx).WithError(err).WithFields(logrus.Fields{
							"sender":  t.Origin,
							"user_id": userID,
							"room_id": roomID,
							"events":  mread.EventIDs,
						}).Error("Failed to send receipt event to edu server")
						continue
					}
				}
			}
		default:
			util.GetLogger(ctx).WithField("type", e.Type).Debug("Unhandled EDU")
		}
	}
}

// processReceiptEvent sends receipt events to the edu server
func (t *txnReq) processReceiptEvent(ctx context.Context,
	userID, roomID, receiptType string,
	timestamp gomatrixserverlib.Timestamp,
	eventIDs []string,
) error {
	// store every event
	for _, eventID := range eventIDs {
		req := eduserverAPI.InputReceiptEventRequest{
			InputReceiptEvent: eduserverAPI.InputReceiptEvent{
				UserID:    userID,
				RoomID:    roomID,
				EventID:   eventID,
				Type:      receiptType,
				Timestamp: timestamp,
			},
		}
		resp := eduserverAPI.InputReceiptEventResponse{}
		if err := t.eduAPI.InputReceiptEvent(ctx, &req, &resp); err != nil {
			return fmt.Errorf("unable to set receipt event: %w", err)
		}
	}

	return nil
}

func (t *txnReq) processDeviceListUpdate(ctx context.Context, e gomatrixserverlib.EDU) {
	var payload gomatrixserverlib.DeviceListUpdateEvent
	if err := json.Unmarshal(e.Content, &payload); err != nil {
		util.GetLogger(ctx).WithError(err).Error("Failed to unmarshal device list update event")
		return
	}
	var inputRes keyapi.InputDeviceListUpdateResponse
	t.keyAPI.InputDeviceListUpdate(context.Background(), &keyapi.InputDeviceListUpdateRequest{
		Event: payload,
	}, &inputRes)
	if inputRes.Error != nil {
		util.GetLogger(ctx).WithError(inputRes.Error).WithField("user_id", payload.UserID).Error("failed to InputDeviceListUpdate")
	}
}

func (t *txnReq) getServers(ctx context.Context, roomID string) []gomatrixserverlib.ServerName {
	servers := []gomatrixserverlib.ServerName{t.Origin}
	serverReq := &api.QueryServerJoinedToRoomRequest{
		RoomID: roomID,
	}
	serverRes := &api.QueryServerJoinedToRoomResponse{}
	if err := t.rsAPI.QueryServerJoinedToRoom(ctx, serverReq, serverRes); err == nil {
		servers = append(servers, serverRes.ServerNames...)
		util.GetLogger(ctx).Infof("Found %d server(s) to query for missing events in %q", len(servers), roomID)
	}
	return servers
}

func (t *txnReq) processEvent(ctx context.Context, e *gomatrixserverlib.Event) error {
	logger := util.GetLogger(ctx).WithField("event_id", e.EventID()).WithField("room_id", e.RoomID())

	// Work out if the roomserver knows everything it needs to know to auth
	// the event. This includes the prev_events and auth_events.
	// NOTE! This is going to include prev_events that have an empty state
	// snapshot. This is because we will need to re-request the event, and
	// it's /state_ids, in order for it to exist in the roomserver correctly
	// before the roomserver tries to work out
	stateReq := api.QueryMissingAuthPrevEventsRequest{
		RoomID:       e.RoomID(),
		AuthEventIDs: e.AuthEventIDs(),
		PrevEventIDs: e.PrevEventIDs(),
	}
	var stateResp api.QueryMissingAuthPrevEventsResponse
	if err := t.rsAPI.QueryMissingAuthPrevEvents(ctx, &stateReq, &stateResp); err != nil {
		return fmt.Errorf("t.rsAPI.QueryMissingAuthPrevEvents: %w", err)
	}

	if !stateResp.RoomExists {
		// TODO: When synapse receives a message for a room it is not in it
		// asks the remote server for the state of the room so that it can
		// check if the remote server knows of a join "m.room.member" event
		// that this server is unaware of.
		// However generally speaking we should reject events for rooms we
		// aren't a member of.
		return roomNotFoundError{e.RoomID()}
	}

	if len(stateResp.MissingAuthEventIDs) > 0 {
		logger.Infof("Event refers to %d unknown auth_events", len(stateResp.MissingAuthEventIDs))
		if err := t.retrieveMissingAuthEvents(ctx, e, &stateResp); err != nil {
			return fmt.Errorf("t.retrieveMissingAuthEvents: %w", err)
		}
	}

	if len(stateResp.MissingPrevEventIDs) > 0 {
		logger.Infof("Event refers to %d unknown prev_events", len(stateResp.MissingPrevEventIDs))
		return t.processEventWithMissingState(ctx, e, stateResp.RoomVersion)
	}

	// pass the event to the roomserver which will do auth checks
	// If the event fail auth checks, gmsl.NotAllowed error will be returned which we be silently
	// discarded by the caller of this function
	return api.SendEvents(
		context.Background(),
		t.rsAPI,
		api.KindNew,
		[]*gomatrixserverlib.HeaderedEvent{
			e.Headered(stateResp.RoomVersion),
		},
		api.DoNotSendToOtherServers,
		nil,
	)
}

func (t *txnReq) retrieveMissingAuthEvents(
	ctx context.Context, e *gomatrixserverlib.Event, stateResp *api.QueryMissingAuthPrevEventsResponse,
) error {
	logger := util.GetLogger(ctx).WithField("event_id", e.EventID()).WithField("room_id", e.RoomID())

	missingAuthEvents := make(map[string]struct{})
	for _, missingAuthEventID := range stateResp.MissingAuthEventIDs {
		missingAuthEvents[missingAuthEventID] = struct{}{}
	}

	servers := t.getServers(ctx, e.RoomID())
	if len(servers) > 5 {
		servers = servers[:5]
	}
withNextEvent:
	for missingAuthEventID := range missingAuthEvents {
	withNextServer:
		for _, server := range servers {
			logger.Infof("Retrieving missing auth event %q from %q", missingAuthEventID, server)
			tx, err := t.federation.GetEvent(ctx, server, missingAuthEventID)
			if err != nil {
				logger.WithError(err).Warnf("Failed to retrieve auth event %q", missingAuthEventID)
				continue withNextServer
			}
			ev, err := gomatrixserverlib.NewEventFromUntrustedJSON(tx.PDUs[0], stateResp.RoomVersion)
			if err != nil {
				logger.WithError(err).Warnf("Failed to unmarshal auth event %q", missingAuthEventID)
				continue withNextServer
			}
			if err = api.SendInputRoomEvents(
				context.Background(),
				t.rsAPI,
				[]api.InputRoomEvent{
					{
						Kind:         api.KindOutlier,
						Event:        ev.Headered(stateResp.RoomVersion),
						AuthEventIDs: ev.AuthEventIDs(),
						SendAsServer: api.DoNotSendToOtherServers,
					},
				},
			); err != nil {
				return fmt.Errorf("api.SendEvents: %w", err)
			}
			delete(missingAuthEvents, missingAuthEventID)
			continue withNextEvent
		}
	}

	if missing := len(missingAuthEvents); missing > 0 {
		return fmt.Errorf("Event refers to %d auth_events which we failed to fetch", missing)
	}
	return nil
}

func checkAllowedByState(e *gomatrixserverlib.Event, stateEvents []*gomatrixserverlib.Event) error {
	authUsingState := gomatrixserverlib.NewAuthEvents(nil)
	for i := range stateEvents {
		err := authUsingState.AddEvent(stateEvents[i])
		if err != nil {
			return err
		}
	}
	return gomatrixserverlib.Allowed(e, &authUsingState)
}

// nolint:gocyclo
func (t *txnReq) processEventWithMissingState(ctx context.Context, e *gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion) error {
	// Do this with a fresh context, so that we keep working even if the
	// original request times out. With any luck, by the time the remote
	// side retries, we'll have fetched the missing state.
	gmectx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()
	// We are missing the previous events for this events.
	// This means that there is a gap in our view of the history of the
	// room. There two ways that we can handle such a gap:
	//   1) We can fill in the gap using /get_missing_events
	//   2) We can leave the gap and request the state of the room at
	//      this event from the remote server using either /state_ids
	//      or /state.
	// Synapse will attempt to do 1 and if that fails or if the gap is
	// too large then it will attempt 2.
	// Synapse will use /state_ids if possible since usually the state
	// is largely unchanged and it is more efficient to fetch a list of
	// event ids and then use /event to fetch the individual events.
	// However not all version of synapse support /state_ids so you may
	// need to fallback to /state.

	// Attempt to fill in the gap using /get_missing_events
	// This will either:
	// - fill in the gap completely then process event `e` returning no backwards extremity
	// - fail to fill in the gap and tell us to terminate the transaction err=not nil
	// - fail to fill in the gap and tell us to fetch state at the new backwards extremity, and to not terminate the transaction
	newEvents, err := t.getMissingEvents(gmectx, e, roomVersion)
	if err != nil {
		return err
	}
	if len(newEvents) == 0 {
		return nil
	}

	backwardsExtremity := newEvents[0]
	newEvents = newEvents[1:]

	type respState struct {
		// A snapshot is considered trustworthy if it came from our own roomserver.
		// That's because the state will have been through state resolution once
		// already in QueryStateAfterEvent.
		trustworthy bool
		*gomatrixserverlib.RespState
	}

	// at this point we know we're going to have a gap: we need to work out the room state at the new backwards extremity.
	// Therefore, we cannot just query /state_ids with this event to get the state before. Instead, we need to query
	// the state AFTER all the prev_events for this event, then apply state resolution to that to get the state before the event.
	var states []*respState
	for _, prevEventID := range backwardsExtremity.PrevEventIDs() {
		// Look up what the state is after the backward extremity. This will either
		// come from the roomserver, if we know all the required events, or it will
		// come from a remote server via /state_ids if not.
		prevState, trustworthy, lerr := t.lookupStateAfterEvent(gmectx, roomVersion, backwardsExtremity.RoomID(), prevEventID)
		if lerr != nil {
			util.GetLogger(ctx).WithError(lerr).Errorf("Failed to lookup state after prev_event: %s", prevEventID)
			return lerr
		}
		// Append the state onto the collected state. We'll run this through the
		// state resolution next.
		states = append(states, &respState{trustworthy, prevState})
	}

	// Now that we have collected all of the state from the prev_events, we'll
	// run the state through the appropriate state resolution algorithm for the
	// room if needed. This does a couple of things:
	// 1. Ensures that the state is deduplicated fully for each state-key tuple
	// 2. Ensures that we pick the latest events from both sets, in the case that
	//    one of the prev_events is quite a bit older than the others
	resolvedState := &gomatrixserverlib.RespState{}
	switch len(states) {
	case 0:
		extremityIsCreate := backwardsExtremity.Type() == gomatrixserverlib.MRoomCreate && backwardsExtremity.StateKeyEquals("")
		if !extremityIsCreate {
			// There are no previous states and this isn't the beginning of the
			// room - this is an error condition!
			util.GetLogger(ctx).Errorf("Failed to lookup any state after prev_events")
			return fmt.Errorf("expected %d states but got %d", len(backwardsExtremity.PrevEventIDs()), len(states))
		}
	case 1:
		// There's only one previous state - if it's trustworthy (came from a
		// local state snapshot which will already have been through state res),
		// use it as-is. There's no point in resolving it again.
		if states[0].trustworthy {
			resolvedState = states[0].RespState
			break
		}
		// Otherwise, if it isn't trustworthy (came from federation), run it through
		// state resolution anyway for safety, in case there are duplicates.
		fallthrough
	default:
		respStates := make([]*gomatrixserverlib.RespState, len(states))
		for i := range states {
			respStates[i] = states[i].RespState
		}
		// There's more than one previous state - run them all through state res
		resolvedState, err = t.resolveStatesAndCheck(gmectx, roomVersion, respStates, backwardsExtremity)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Errorf("Failed to resolve state conflicts for event %s", backwardsExtremity.EventID())
			return err
		}
	}

	// First of all, send the backward extremity into the roomserver with the
	// newly resolved state. This marks the "oldest" point in the backfill and
	// sets the baseline state for any new events after this.
	err = api.SendEventWithState(
		context.Background(),
		t.rsAPI,
		api.KindOld,
		resolvedState,
		backwardsExtremity.Headered(roomVersion),
		t.haveEventIDs(),
	)
	if err != nil {
		return fmt.Errorf("api.SendEventWithState: %w", err)
	}

	// Then send all of the newer backfilled events, of which will all be newer
	// than the backward extremity, into the roomserver without state. This way
	// they will automatically fast-forward based on the room state at the
	// extremity in the last step.
	headeredNewEvents := make([]*gomatrixserverlib.HeaderedEvent, len(newEvents))
	for i, newEvent := range newEvents {
		headeredNewEvents[i] = newEvent.Headered(roomVersion)
	}
	if err = api.SendEvents(
		context.Background(),
		t.rsAPI,
		api.KindOld,
		append(headeredNewEvents, e.Headered(roomVersion)),
		api.DoNotSendToOtherServers,
		nil,
	); err != nil {
		return fmt.Errorf("api.SendEvents: %w", err)
	}

	return nil
}

// lookupStateAfterEvent returns the room state after `eventID`, which is the state before eventID with the state of `eventID` (if it's a state event)
// added into the mix.
func (t *txnReq) lookupStateAfterEvent(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string) (*gomatrixserverlib.RespState, bool, error) {
	// try doing all this locally before we resort to querying federation
	respState := t.lookupStateAfterEventLocally(ctx, roomID, eventID)
	if respState != nil {
		return respState, true, nil
	}

	respState, err := t.lookupStateBeforeEvent(ctx, roomVersion, roomID, eventID)
	if err != nil {
		return nil, false, fmt.Errorf("t.lookupStateBeforeEvent: %w", err)
	}

	servers := t.getServers(ctx, roomID)
	if len(servers) > 5 {
		servers = servers[:5]
	}

	// fetch the event we're missing and add it to the pile
	h, err := t.lookupEvent(ctx, roomVersion, eventID, false, servers)
	switch err.(type) {
	case verifySigError:
		return respState, false, nil
	case nil:
		// do nothing
	default:
		return nil, false, fmt.Errorf("t.lookupEvent: %w", err)
	}
	t.haveEvents[h.EventID()] = h
	if h.StateKey() != nil {
		addedToState := false
		for i := range respState.StateEvents {
			se := respState.StateEvents[i]
			if se.Type() == h.Type() && se.StateKeyEquals(*h.StateKey()) {
				respState.StateEvents[i] = h.Unwrap()
				addedToState = true
				break
			}
		}
		if !addedToState {
			respState.StateEvents = append(respState.StateEvents, h.Unwrap())
		}
	}

	return respState, false, nil
}

func (t *txnReq) lookupStateAfterEventLocally(ctx context.Context, roomID, eventID string) *gomatrixserverlib.RespState {
	var res api.QueryStateAfterEventsResponse
	err := t.rsAPI.QueryStateAfterEvents(ctx, &api.QueryStateAfterEventsRequest{
		RoomID:       roomID,
		PrevEventIDs: []string{eventID},
	}, &res)
	if err != nil || !res.PrevEventsExist {
		util.GetLogger(ctx).WithError(err).Warnf("failed to query state after %s locally", eventID)
		return nil
	}
	for i, ev := range res.StateEvents {
		t.haveEvents[ev.EventID()] = res.StateEvents[i]
	}
	var authEvents []*gomatrixserverlib.Event
	missingAuthEvents := make(map[string]bool)
	for _, ev := range res.StateEvents {
		for _, ae := range ev.AuthEventIDs() {
			aev, ok := t.haveEvents[ae]
			if ok {
				authEvents = append(authEvents, aev.Unwrap())
			} else {
				missingAuthEvents[ae] = true
			}
		}
	}
	// QueryStateAfterEvents does not return the auth events, so fetch them now. We know the roomserver has them else it wouldn't
	// have stored the event.
	var missingEventList []string
	for evID := range missingAuthEvents {
		missingEventList = append(missingEventList, evID)
	}
	queryReq := api.QueryEventsByIDRequest{
		EventIDs: missingEventList,
	}
	util.GetLogger(ctx).Infof("Fetching missing auth events: %v", missingEventList)
	var queryRes api.QueryEventsByIDResponse
	if err = t.rsAPI.QueryEventsByID(ctx, &queryReq, &queryRes); err != nil {
		return nil
	}
	for i := range queryRes.Events {
		evID := queryRes.Events[i].EventID()
		t.haveEvents[evID] = queryRes.Events[i]
		authEvents = append(authEvents, queryRes.Events[i].Unwrap())
	}

	evs := gomatrixserverlib.UnwrapEventHeaders(res.StateEvents)
	return &gomatrixserverlib.RespState{
		StateEvents: evs,
		AuthEvents:  authEvents,
	}
}

// lookuptStateBeforeEvent returns the room state before the event e, which is just /state_ids and/or /state depending on what
// the server supports.
func (t *txnReq) lookupStateBeforeEvent(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string) (
	*gomatrixserverlib.RespState, error) {

	util.GetLogger(ctx).Infof("lookupStateBeforeEvent %s", eventID)

	// Attempt to fetch the missing state using /state_ids and /events
	return t.lookupMissingStateViaStateIDs(ctx, roomID, eventID, roomVersion)
}

func (t *txnReq) resolveStatesAndCheck(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, states []*gomatrixserverlib.RespState, backwardsExtremity *gomatrixserverlib.Event) (*gomatrixserverlib.RespState, error) {
	var authEventList []*gomatrixserverlib.Event
	var stateEventList []*gomatrixserverlib.Event
	for _, state := range states {
		authEventList = append(authEventList, state.AuthEvents...)
		stateEventList = append(stateEventList, state.StateEvents...)
	}
	resolvedStateEvents, err := gomatrixserverlib.ResolveConflicts(roomVersion, stateEventList, authEventList)
	if err != nil {
		return nil, err
	}
	// apply the current event
retryAllowedState:
	if err = checkAllowedByState(backwardsExtremity, resolvedStateEvents); err != nil {
		switch missing := err.(type) {
		case gomatrixserverlib.MissingAuthEventError:
			servers := t.getServers(ctx, backwardsExtremity.RoomID())
			if len(servers) > 5 {
				servers = servers[:5]
			}
			h, err2 := t.lookupEvent(ctx, roomVersion, missing.AuthEventID, true, servers)
			switch err2.(type) {
			case verifySigError:
				return &gomatrixserverlib.RespState{
					AuthEvents:  authEventList,
					StateEvents: resolvedStateEvents,
				}, nil
			case nil:
				// do nothing
			default:
				return nil, fmt.Errorf("missing auth event %s and failed to look it up: %w", missing.AuthEventID, err2)
			}
			util.GetLogger(ctx).Infof("fetched event %s", missing.AuthEventID)
			resolvedStateEvents = append(resolvedStateEvents, h.Unwrap())
			goto retryAllowedState
		default:
		}
		return nil, err
	}
	return &gomatrixserverlib.RespState{
		AuthEvents:  authEventList,
		StateEvents: resolvedStateEvents,
	}, nil
}

// getMissingEvents returns a nil backwardsExtremity if missing events were fetched and handled, else returns the new backwards extremity which we should
// begin from. Returns an error only if we should terminate the transaction which initiated /get_missing_events
// This function recursively calls txnReq.processEvent with the missing events, which will be processed before this function returns.
// This means that we may recursively call this function, as we spider back up prev_events.
// nolint:gocyclo
func (t *txnReq) getMissingEvents(ctx context.Context, e *gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion) (newEvents []*gomatrixserverlib.Event, err error) {
	logger := util.GetLogger(ctx).WithField("event_id", e.EventID()).WithField("room_id", e.RoomID())
	needed := gomatrixserverlib.StateNeededForAuth([]*gomatrixserverlib.Event{e})
	// query latest events (our trusted forward extremities)
	req := api.QueryLatestEventsAndStateRequest{
		RoomID:       e.RoomID(),
		StateToFetch: needed.Tuples(),
	}
	var res api.QueryLatestEventsAndStateResponse
	if err = t.rsAPI.QueryLatestEventsAndState(ctx, &req, &res); err != nil {
		logger.WithError(err).Warn("Failed to query latest events")
		return nil, err
	}
	latestEvents := make([]string, len(res.LatestEvents))
	for i := range res.LatestEvents {
		latestEvents[i] = res.LatestEvents[i].EventID
	}

	servers := []gomatrixserverlib.ServerName{t.Origin}
	serverReq := &api.QueryServerJoinedToRoomRequest{
		RoomID: e.RoomID(),
	}
	serverRes := &api.QueryServerJoinedToRoomResponse{}
	if err = t.rsAPI.QueryServerJoinedToRoom(ctx, serverReq, serverRes); err == nil {
		servers = append(servers, serverRes.ServerNames...)
		logger.Infof("Found %d server(s) to query for missing events", len(servers))
	}

	var missingResp *gomatrixserverlib.RespMissingEvents
	for _, server := range servers {
		var m gomatrixserverlib.RespMissingEvents
		if m, err = t.federation.LookupMissingEvents(ctx, server, e.RoomID(), gomatrixserverlib.MissingEvents{
			Limit: 20,
			// The latest event IDs that the sender already has. These are skipped when retrieving the previous events of latest_events.
			EarliestEvents: latestEvents,
			// The event IDs to retrieve the previous events for.
			LatestEvents: []string{e.EventID()},
		}, roomVersion); err == nil {
			missingResp = &m
			break
		} else {
			logger.WithError(err).Errorf("%s pushed us an event but %q did not respond to /get_missing_events", t.Origin, server)
		}
	}

	if missingResp == nil {
		logger.WithError(err).Errorf(
			"%s pushed us an event but %d server(s) couldn't give us details about prev_events via /get_missing_events - dropping this event until it can",
			t.Origin, len(servers),
		)
		return nil, missingPrevEventsError{
			eventID: e.EventID(),
			err:     err,
		}
	}

	// security: how we handle failures depends on whether or not this event will become the new forward extremity for the room.
	// There's 2 scenarios to consider:
	// - Case A: We got pushed an event and are now fetching missing prev_events. (isInboundTxn=true)
	// - Case B: We are fetching missing prev_events already and now fetching some more  (isInboundTxn=false)
	// In Case B, we know for sure that the event we are currently processing will not become the new forward extremity for the room,
	// as it was called in response to an inbound txn which had it as a prev_event.
	// In Case A, the event is a forward extremity, and could eventually become the _only_ forward extremity in the room. This is bad
	// because it means we would trust the state at that event to be the state for the entire room, and allows rooms to be hijacked.
	// https://github.com/matrix-org/synapse/pull/3456
	// https://github.com/matrix-org/synapse/blob/229eb81498b0fe1da81e9b5b333a0285acde9446/synapse/handlers/federation.py#L335
	// For now, we do not allow Case B, so reject the event.
	logger.Infof("get_missing_events returned %d events", len(missingResp.Events))

	// topologically sort and sanity check that we are making forward progress
	newEvents = gomatrixserverlib.ReverseTopologicalOrdering(missingResp.Events, gomatrixserverlib.TopologicalOrderByPrevEvents)
	shouldHaveSomeEventIDs := e.PrevEventIDs()
	hasPrevEvent := false
Event:
	for _, pe := range shouldHaveSomeEventIDs {
		for _, ev := range newEvents {
			if ev.EventID() == pe {
				hasPrevEvent = true
				break Event
			}
		}
	}
	if !hasPrevEvent {
		err = fmt.Errorf("called /get_missing_events but server %s didn't return any prev_events with IDs %v", t.Origin, shouldHaveSomeEventIDs)
		logger.WithError(err).Errorf(
			"%s pushed us an event but couldn't give us details about prev_events via /get_missing_events - dropping this event until it can",
			t.Origin,
		)
		return nil, missingPrevEventsError{
			eventID: e.EventID(),
			err:     err,
		}
	}

	// we processed everything!
	return newEvents, nil
}

func (t *txnReq) lookupMissingStateViaState(ctx context.Context, roomID, eventID string, roomVersion gomatrixserverlib.RoomVersion) (
	respState *gomatrixserverlib.RespState, err error) {
	state, err := t.federation.LookupState(ctx, t.Origin, roomID, eventID, roomVersion)
	if err != nil {
		return nil, err
	}
	// Check that the returned state is valid.
	if err := state.Check(ctx, t.keys, nil); err != nil {
		return nil, err
	}
	return &state, nil
}

// nolint:gocyclo
func (t *txnReq) lookupMissingStateViaStateIDs(ctx context.Context, roomID, eventID string, roomVersion gomatrixserverlib.RoomVersion) (
	*gomatrixserverlib.RespState, error) {
	util.GetLogger(ctx).Infof("lookupMissingStateViaStateIDs %s", eventID)
	// fetch the state event IDs at the time of the event
	stateIDs, err := t.federation.LookupStateIDs(ctx, t.Origin, roomID, eventID)
	if err != nil {
		return nil, err
	}
	// work out which auth/state IDs are missing
	wantIDs := append(stateIDs.StateEventIDs, stateIDs.AuthEventIDs...)
	missing := make(map[string]bool)
	var missingEventList []string
	for _, sid := range wantIDs {
		if _, ok := t.haveEvents[sid]; !ok {
			if !missing[sid] {
				missing[sid] = true
				missingEventList = append(missingEventList, sid)
			}
		}
	}

	// fetch as many as we can from the roomserver
	queryReq := api.QueryEventsByIDRequest{
		EventIDs: missingEventList,
	}
	var queryRes api.QueryEventsByIDResponse
	if err = t.rsAPI.QueryEventsByID(ctx, &queryReq, &queryRes); err != nil {
		return nil, err
	}
	for i := range queryRes.Events {
		evID := queryRes.Events[i].EventID()
		t.haveEvents[evID] = queryRes.Events[i]
		if missing[evID] {
			delete(missing, evID)
		}
	}

	concurrentRequests := 8
	missingCount := len(missing)

	// If over 50% of the auth/state events from /state_ids are missing
	// then we'll just call /state instead, otherwise we'll just end up
	// hammering the remote side with /event requests unnecessarily.
	if missingCount > concurrentRequests && missingCount > len(wantIDs)/2 {
		util.GetLogger(ctx).WithFields(logrus.Fields{
			"missing":           missingCount,
			"event_id":          eventID,
			"room_id":           roomID,
			"total_state":       len(stateIDs.StateEventIDs),
			"total_auth_events": len(stateIDs.AuthEventIDs),
		}).Info("Fetching all state at event")
		return t.lookupMissingStateViaState(ctx, roomID, eventID, roomVersion)
	}

	if missingCount > 0 {
		util.GetLogger(ctx).WithFields(logrus.Fields{
			"missing":             missingCount,
			"event_id":            eventID,
			"room_id":             roomID,
			"total_state":         len(stateIDs.StateEventIDs),
			"total_auth_events":   len(stateIDs.AuthEventIDs),
			"concurrent_requests": concurrentRequests,
		}).Info("Fetching missing state at event")

		// Get a list of servers to fetch from.
		servers := t.getServers(ctx, roomID)
		if len(servers) > 5 {
			servers = servers[:5]
		}

		// Create a queue containing all of the missing event IDs that we want
		// to retrieve.
		pending := make(chan string, missingCount)
		for missingEventID := range missing {
			pending <- missingEventID
		}
		close(pending)

		// Define how many workers we should start to do this.
		if missingCount < concurrentRequests {
			concurrentRequests = missingCount
		}

		// Create the wait group.
		var fetchgroup sync.WaitGroup
		fetchgroup.Add(concurrentRequests)

		// This is the only place where we'll write to t.haveEvents from
		// multiple goroutines, and everywhere else is blocked on this
		// synchronous function anyway.
		var haveEventsMutex sync.Mutex

		// Define what we'll do in order to fetch the missing event ID.
		fetch := func(missingEventID string) {
			var h *gomatrixserverlib.HeaderedEvent
			h, err = t.lookupEvent(ctx, roomVersion, missingEventID, false, servers)
			switch err.(type) {
			case verifySigError:
				return
			case nil:
				break
			default:
				util.GetLogger(ctx).WithFields(logrus.Fields{
					"event_id": missingEventID,
					"room_id":  roomID,
				}).Info("Failed to fetch missing event")
				return
			}
			haveEventsMutex.Lock()
			t.haveEvents[h.EventID()] = h
			haveEventsMutex.Unlock()
		}

		// Create the worker.
		worker := func(ch <-chan string) {
			defer fetchgroup.Done()
			for missingEventID := range ch {
				fetch(missingEventID)
			}
		}

		// Start the workers.
		for i := 0; i < concurrentRequests; i++ {
			go worker(pending)
		}

		// Wait for the workers to finish.
		fetchgroup.Wait()
	}

	resp, err := t.createRespStateFromStateIDs(stateIDs)
	return resp, err
}

func (t *txnReq) createRespStateFromStateIDs(stateIDs gomatrixserverlib.RespStateIDs) (
	*gomatrixserverlib.RespState, error) { // nolint:unparam
	// create a RespState response using the response to /state_ids as a guide
	respState := gomatrixserverlib.RespState{}

	for i := range stateIDs.StateEventIDs {
		ev, ok := t.haveEvents[stateIDs.StateEventIDs[i]]
		if !ok {
			logrus.Warnf("Missing state event in createRespStateFromStateIDs: %s", stateIDs.StateEventIDs[i])
			continue
		}
		respState.StateEvents = append(respState.StateEvents, ev.Unwrap())
	}
	for i := range stateIDs.AuthEventIDs {
		ev, ok := t.haveEvents[stateIDs.AuthEventIDs[i]]
		if !ok {
			logrus.Warnf("Missing auth event in createRespStateFromStateIDs: %s", stateIDs.AuthEventIDs[i])
			continue
		}
		respState.AuthEvents = append(respState.AuthEvents, ev.Unwrap())
	}
	// We purposefully do not do auth checks on the returned events, as they will still
	// be processed in the exact same way, just as a 'rejected' event
	// TODO: Add a field to HeaderedEvent to indicate if the event is rejected.
	return &respState, nil
}

func (t *txnReq) lookupEvent(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, missingEventID string, localFirst bool, servers []gomatrixserverlib.ServerName) (*gomatrixserverlib.HeaderedEvent, error) {
	if localFirst {
		// fetch from the roomserver
		queryReq := api.QueryEventsByIDRequest{
			EventIDs: []string{missingEventID},
		}
		var queryRes api.QueryEventsByIDResponse
		if err := t.rsAPI.QueryEventsByID(ctx, &queryReq, &queryRes); err != nil {
			util.GetLogger(ctx).Warnf("Failed to query roomserver for missing event %s: %s - falling back to remote", missingEventID, err)
		} else if len(queryRes.Events) == 1 {
			return queryRes.Events[0], nil
		}
	}
	var event *gomatrixserverlib.Event
	found := false
	for _, serverName := range servers {
		txn, err := t.federation.GetEvent(ctx, serverName, missingEventID)
		if err != nil || len(txn.PDUs) == 0 {
			util.GetLogger(ctx).WithError(err).WithField("event_id", missingEventID).Warn("Failed to get missing /event for event ID")
			continue
		}
		event, err = gomatrixserverlib.NewEventFromUntrustedJSON(txn.PDUs[0], roomVersion)
		if err != nil {
			util.GetLogger(ctx).WithError(err).WithField("event_id", missingEventID).Warnf("Transaction: Failed to parse event JSON of event")
			continue
		}
		found = true
		break
	}
	if !found {
		util.GetLogger(ctx).WithField("event_id", missingEventID).Warnf("Failed to get missing /event for event ID from %d server(s)", len(servers))
		return nil, fmt.Errorf("wasn't able to find event via %d server(s)", len(servers))
	}
	if err := gomatrixserverlib.VerifyAllEventSignatures(ctx, []*gomatrixserverlib.Event{event}, t.keys); err != nil {
		util.GetLogger(ctx).WithError(err).Warnf("Transaction: Couldn't validate signature of event %q", event.EventID())
		return nil, verifySigError{event.EventID(), err}
	}
	h := event.Headered(roomVersion)
	t.newEventsMutex.Lock()
	t.newEvents[h.EventID()] = true
	t.newEventsMutex.Unlock()
	return h, nil
}
