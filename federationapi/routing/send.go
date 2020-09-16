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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	eduserverAPI "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal/config"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/roomserver/api"
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

	util.GetLogger(httpReq.Context()).Infof("Received transaction %q containing %d PDUs, %d EDUs", txnID, len(t.PDUs), len(t.EDUs))

	resp, jsonErr := t.processTransaction(httpReq.Context())
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
	newEvents map[string]bool
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

	pdus := []gomatrixserverlib.HeaderedEvent{}
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
		if err = gomatrixserverlib.VerifyAllEventSignatures(ctx, []gomatrixserverlib.Event{event}, t.keys); err != nil {
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
		if err := t.processEvent(ctx, e.Unwrap(), true); err != nil {
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
				_, rejected := err.(*gomatrixserverlib.NotAllowed)
				errMsg := err.Error()
				if rejected {
					errMsg = ""
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
	switch err.(type) {
	case roomNotFoundError:
	case *gomatrixserverlib.NotAllowed:
	case missingPrevEventsError:
	default:
		switch err {
		case context.Canceled:
		case context.DeadlineExceeded:
		default:
			return true
		}
	}
	return false
}

type roomNotFoundError struct {
	roomID string
}
type unmarshalError struct {
	err error
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
func (e unmarshalError) Error() string    { return fmt.Sprintf("unable to parse event: %s", e.err) }
func (e verifySigError) Error() string {
	return fmt.Sprintf("unable to verify signature of event %q: %s", e.eventID, e.err)
}
func (e missingPrevEventsError) Error() string {
	return fmt.Sprintf("unable to get prev_events for event %q: %s", e.eventID, e.err)
}

func (t *txnReq) haveEventIDs() map[string]bool {
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
		default:
			util.GetLogger(ctx).WithField("type", e.Type).Debug("Unhandled EDU")
		}
	}
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

func (t *txnReq) processEvent(ctx context.Context, e gomatrixserverlib.Event, isInboundTxn bool) error {
	prevEventIDs := e.PrevEventIDs()

	// Fetch the state needed to authenticate the event.
	needed := gomatrixserverlib.StateNeededForAuth([]gomatrixserverlib.Event{e})
	stateReq := api.QueryStateAfterEventsRequest{
		RoomID:       e.RoomID(),
		PrevEventIDs: prevEventIDs,
		StateToFetch: needed.Tuples(),
	}
	var stateResp api.QueryStateAfterEventsResponse
	if err := t.rsAPI.QueryStateAfterEvents(ctx, &stateReq, &stateResp); err != nil {
		return err
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

	if !stateResp.PrevEventsExist {
		return t.processEventWithMissingState(ctx, e, stateResp.RoomVersion, isInboundTxn)
	}

	// pass the event to the roomserver which will do auth checks
	// If the event fail auth checks, gmsl.NotAllowed error will be returned which we be silently
	// discarded by the caller of this function
	return api.SendEvents(
		context.Background(),
		t.rsAPI,
		[]gomatrixserverlib.HeaderedEvent{
			e.Headered(stateResp.RoomVersion),
		},
		api.DoNotSendToOtherServers,
		nil,
	)
}

func checkAllowedByState(e gomatrixserverlib.Event, stateEvents []gomatrixserverlib.Event) error {
	authUsingState := gomatrixserverlib.NewAuthEvents(nil)
	for i := range stateEvents {
		err := authUsingState.AddEvent(&stateEvents[i])
		if err != nil {
			return err
		}
	}
	return gomatrixserverlib.Allowed(e, &authUsingState)
}

func (t *txnReq) processEventWithMissingState(ctx context.Context, e gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion, isInboundTxn bool) error {
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
	backwardsExtremity, err := t.getMissingEvents(gmectx, e, roomVersion, isInboundTxn)
	if err != nil {
		return err
	}
	if backwardsExtremity == nil {
		// we filled in the gap!
		return nil
	}

	// at this point we know we're going to have a gap: we need to work out the room state at the new backwards extremity.
	// security: we have to do state resolution on the new backwards extremity (TODO: WHY)
	// Therefore, we cannot just query /state_ids with this event to get the state before. Instead, we need to query
	// the state AFTER all the prev_events for this event, then apply state resolution to that to get the state before the event.
	var states []*gomatrixserverlib.RespState
	needed := gomatrixserverlib.StateNeededForAuth([]gomatrixserverlib.Event{*backwardsExtremity}).Tuples()
	for _, prevEventID := range backwardsExtremity.PrevEventIDs() {
		var prevState *gomatrixserverlib.RespState
		prevState, err = t.lookupStateAfterEvent(gmectx, roomVersion, backwardsExtremity.RoomID(), prevEventID, needed)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Errorf("Failed to lookup state after prev_event: %s", prevEventID)
			return err
		}
		states = append(states, prevState)
	}
	resolvedState, err := t.resolveStatesAndCheck(gmectx, roomVersion, states, backwardsExtremity)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Errorf("Failed to resolve state conflicts for event %s", backwardsExtremity.EventID())
		return err
	}

	// pass the event along with the state to the roomserver using a background context so we don't
	// needlessly expire
	return api.SendEventWithState(context.Background(), t.rsAPI, resolvedState, e.Headered(roomVersion), t.haveEventIDs())
}

// lookupStateAfterEvent returns the room state after `eventID`, which is the state before eventID with the state of `eventID` (if it's a state event)
// added into the mix.
func (t *txnReq) lookupStateAfterEvent(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, roomID, eventID string, needed []gomatrixserverlib.StateKeyTuple) (*gomatrixserverlib.RespState, error) {
	// try doing all this locally before we resort to querying federation
	respState := t.lookupStateAfterEventLocally(ctx, roomID, eventID, needed)
	if respState != nil {
		return respState, nil
	}

	respState, err := t.lookupStateBeforeEvent(ctx, roomVersion, roomID, eventID)
	if err != nil {
		return nil, err
	}

	// fetch the event we're missing and add it to the pile
	h, err := t.lookupEvent(ctx, roomVersion, eventID, false)
	switch err.(type) {
	case verifySigError:
		return respState, nil
	case nil:
		// do nothing
	default:
		return nil, err
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

	return respState, nil
}

func (t *txnReq) lookupStateAfterEventLocally(ctx context.Context, roomID, eventID string, needed []gomatrixserverlib.StateKeyTuple) *gomatrixserverlib.RespState {
	var res api.QueryStateAfterEventsResponse
	err := t.rsAPI.QueryStateAfterEvents(ctx, &api.QueryStateAfterEventsRequest{
		RoomID:       roomID,
		PrevEventIDs: []string{eventID},
		StateToFetch: needed,
	}, &res)
	if err != nil || !res.PrevEventsExist {
		util.GetLogger(ctx).WithError(err).Warnf("failed to query state after %s locally", eventID)
		return nil
	}
	for i, ev := range res.StateEvents {
		t.haveEvents[ev.EventID()] = &res.StateEvents[i]
	}
	var authEvents []gomatrixserverlib.Event
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
		t.haveEvents[evID] = &queryRes.Events[i]
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
	respState *gomatrixserverlib.RespState, err error) {

	util.GetLogger(ctx).Infof("lookupStateBeforeEvent %s", eventID)

	// Attempt to fetch the missing state using /state_ids and /events
	respState, err = t.lookupMissingStateViaStateIDs(ctx, roomID, eventID, roomVersion)
	if err != nil {
		// Fallback to /state
		util.GetLogger(ctx).WithError(err).Warn("lookupStateBeforeEvent failed to /state_ids, falling back to /state")
		respState, err = t.lookupMissingStateViaState(ctx, roomID, eventID, roomVersion)
	}
	return
}

func (t *txnReq) resolveStatesAndCheck(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, states []*gomatrixserverlib.RespState, backwardsExtremity *gomatrixserverlib.Event) (*gomatrixserverlib.RespState, error) {
	var authEventList []gomatrixserverlib.Event
	var stateEventList []gomatrixserverlib.Event
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
	if err = checkAllowedByState(*backwardsExtremity, resolvedStateEvents); err != nil {
		switch missing := err.(type) {
		case gomatrixserverlib.MissingAuthEventError:
			h, err2 := t.lookupEvent(ctx, roomVersion, missing.AuthEventID, true)
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
// This means that we may recursively call this function, as we spider back up prev_events to the min depth.
func (t *txnReq) getMissingEvents(ctx context.Context, e gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion, isInboundTxn bool) (backwardsExtremity *gomatrixserverlib.Event, err error) {
	if !isInboundTxn {
		// we've recursed here, so just take a state snapshot please!
		return &e, nil
	}
	logger := util.GetLogger(ctx).WithField("event_id", e.EventID()).WithField("room_id", e.RoomID())
	needed := gomatrixserverlib.StateNeededForAuth([]gomatrixserverlib.Event{e})
	// query latest events (our trusted forward extremities)
	req := api.QueryLatestEventsAndStateRequest{
		RoomID:       e.RoomID(),
		StateToFetch: needed.Tuples(),
	}
	var res api.QueryLatestEventsAndStateResponse
	if err = t.rsAPI.QueryLatestEventsAndState(ctx, &req, &res); err != nil {
		logger.WithError(err).Warn("Failed to query latest events")
		return &e, nil
	}
	latestEvents := make([]string, len(res.LatestEvents))
	for i := range res.LatestEvents {
		latestEvents[i] = res.LatestEvents[i].EventID
	}
	// this server just sent us an event for which we do not know its prev_events - ask that server for those prev_events.
	minDepth := int(res.Depth) - 20
	if minDepth < 0 {
		minDepth = 0
	}
	missingResp, err := t.federation.LookupMissingEvents(ctx, t.Origin, e.RoomID(), gomatrixserverlib.MissingEvents{
		Limit: 20,
		// synapse uses the min depth they've ever seen in that room
		MinDepth: minDepth,
		// The latest event IDs that the sender already has. These are skipped when retrieving the previous events of latest_events.
		EarliestEvents: latestEvents,
		// The event IDs to retrieve the previous events for.
		LatestEvents: []string{e.EventID()},
	}, roomVersion)

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
	if err != nil {
		logger.WithError(err).Errorf(
			"%s pushed us an event but couldn't give us details about prev_events via /get_missing_events - dropping this event until it can",
			t.Origin,
		)
		return nil, missingPrevEventsError{
			eventID: e.EventID(),
			err:     err,
		}
	}
	logger.Infof("get_missing_events returned %d events", len(missingResp.Events))

	// topologically sort and sanity check that we are making forward progress
	newEvents := gomatrixserverlib.ReverseTopologicalOrdering(missingResp.Events, gomatrixserverlib.TopologicalOrderByPrevEvents)
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
	// process the missing events then the event which started this whole thing
	for _, ev := range append(newEvents, e) {
		err := t.processEvent(ctx, ev, false)
		if err != nil {
			return nil, err
		}
	}

	// we processed everything!
	return nil, nil
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
		t.haveEvents[evID] = &queryRes.Events[i]
		if missing[evID] {
			delete(missing, evID)
		}
	}

	util.GetLogger(ctx).WithFields(logrus.Fields{
		"missing":           len(missing),
		"event_id":          eventID,
		"room_id":           roomID,
		"total_state":       len(stateIDs.StateEventIDs),
		"total_auth_events": len(stateIDs.AuthEventIDs),
	}).Info("Fetching missing state at event")

	for missingEventID := range missing {
		var h *gomatrixserverlib.HeaderedEvent
		h, err = t.lookupEvent(ctx, roomVersion, missingEventID, false)
		switch err.(type) {
		case verifySigError:
			continue
		case nil:
			// do nothing
		default:
			return nil, err
		}
		t.haveEvents[h.EventID()] = h
	}
	resp, err := t.createRespStateFromStateIDs(stateIDs)
	return resp, err
}

func (t *txnReq) createRespStateFromStateIDs(stateIDs gomatrixserverlib.RespStateIDs) (
	*gomatrixserverlib.RespState, error) {
	// create a RespState response using the response to /state_ids as a guide
	respState := gomatrixserverlib.RespState{
		AuthEvents:  make([]gomatrixserverlib.Event, len(stateIDs.AuthEventIDs)),
		StateEvents: make([]gomatrixserverlib.Event, len(stateIDs.StateEventIDs)),
	}

	for i := range stateIDs.StateEventIDs {
		ev, ok := t.haveEvents[stateIDs.StateEventIDs[i]]
		if !ok {
			return nil, fmt.Errorf("missing state event %s", stateIDs.StateEventIDs[i])
		}
		respState.StateEvents[i] = ev.Unwrap()
	}
	for i := range stateIDs.AuthEventIDs {
		ev, ok := t.haveEvents[stateIDs.AuthEventIDs[i]]
		if !ok {
			return nil, fmt.Errorf("missing auth event %s", stateIDs.AuthEventIDs[i])
		}
		respState.AuthEvents[i] = ev.Unwrap()
	}
	// We purposefully do not do auth checks on the returned events, as they will still
	// be processed in the exact same way, just as a 'rejected' event
	// TODO: Add a field to HeaderedEvent to indicate if the event is rejected.
	return &respState, nil
}

func (t *txnReq) lookupEvent(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, missingEventID string, localFirst bool) (*gomatrixserverlib.HeaderedEvent, error) {
	if localFirst {
		// fetch from the roomserver
		queryReq := api.QueryEventsByIDRequest{
			EventIDs: []string{missingEventID},
		}
		var queryRes api.QueryEventsByIDResponse
		if err := t.rsAPI.QueryEventsByID(ctx, &queryReq, &queryRes); err != nil {
			util.GetLogger(ctx).Warnf("Failed to query roomserver for missing event %s: %s - falling back to remote", missingEventID, err)
		} else if len(queryRes.Events) == 1 {
			return &queryRes.Events[0], nil
		}
	}
	txn, err := t.federation.GetEvent(ctx, t.Origin, missingEventID)
	if err != nil || len(txn.PDUs) == 0 {
		util.GetLogger(ctx).WithError(err).WithField("event_id", missingEventID).Warn("failed to get missing /event for event ID")
		return nil, err
	}
	pdu := txn.PDUs[0]
	var event gomatrixserverlib.Event
	event, err = gomatrixserverlib.NewEventFromUntrustedJSON(pdu, roomVersion)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Warnf("Transaction: Failed to parse event JSON of event %q", event.EventID())
		return nil, unmarshalError{err}
	}
	if err = gomatrixserverlib.VerifyAllEventSignatures(ctx, []gomatrixserverlib.Event{event}, t.keys); err != nil {
		util.GetLogger(ctx).WithError(err).Warnf("Transaction: Couldn't validate signature of event %q", event.EventID())
		return nil, verifySigError{event.EventID(), err}
	}
	h := event.Headered(roomVersion)
	t.newEvents[h.EventID()] = true
	return &h, nil
}
