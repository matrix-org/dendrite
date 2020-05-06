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

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common/config"
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
	cfg *config.Dendrite,
	rsAPI api.RoomserverInternalAPI,
	producer *producers.RoomserverProducer,
	eduProducer *producers.EDUServerProducer,
	keys gomatrixserverlib.KeyRing,
	federation *gomatrixserverlib.FederationClient,
) util.JSONResponse {
	t := txnReq{
		context:     httpReq.Context(),
		rsAPI:       rsAPI,
		producer:    producer,
		eduProducer: eduProducer,
		keys:        keys,
		federation:  federation,
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

	// TODO: Really we should have a function to convert FederationRequest to txnReq
	t.PDUs = txnEvents.PDUs
	t.EDUs = txnEvents.EDUs
	t.Origin = request.Origin()
	t.TransactionID = txnID
	t.Destination = cfg.Matrix.ServerName

	util.GetLogger(httpReq.Context()).Infof("Received transaction %q containing %d PDUs, %d EDUs", txnID, len(t.PDUs), len(t.EDUs))

	resp, err := t.processTransaction()
	switch err.(type) {
	// No error? Great! Send back a 200.
	case nil:
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: resp,
		}
	// Handle known error cases as we will return a 400 error for these.
	case roomNotFoundError:
	case unmarshalError:
	case verifySigError:
	// Handle unknown error cases. Sending 500 errors back should be a last
	// resort as this can make other homeservers back off sending federation
	// events.
	default:
		util.GetLogger(httpReq.Context()).WithError(err).Error("t.processTransaction failed")
		return jsonerror.InternalServerError()
	}
	// Return a 400 error for bad requests as fallen through from above.
	return util.JSONResponse{
		Code: http.StatusBadRequest,
		JSON: jsonerror.BadJSON(err.Error()),
	}
}

type txnReq struct {
	gomatrixserverlib.Transaction
	context     context.Context
	rsAPI       api.RoomserverInternalAPI
	producer    *producers.RoomserverProducer
	eduProducer *producers.EDUServerProducer
	keys        gomatrixserverlib.JSONVerifier
	federation  txnFederationClient
}

// A subset of FederationClient functionality that txn requires. Useful for testing.
type txnFederationClient interface {
	LookupState(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string, roomVersion gomatrixserverlib.RoomVersion) (
		res gomatrixserverlib.RespState, err error,
	)
	LookupStateIDs(ctx context.Context, s gomatrixserverlib.ServerName, roomID string, eventID string) (res gomatrixserverlib.RespStateIDs, err error)
	GetEvent(ctx context.Context, s gomatrixserverlib.ServerName, eventID string) (res gomatrixserverlib.Transaction, err error)
}

func (t *txnReq) processTransaction() (*gomatrixserverlib.RespSend, error) {
	results := make(map[string]gomatrixserverlib.PDUResult)

	var pdus []gomatrixserverlib.HeaderedEvent
	for _, pdu := range t.PDUs {
		var header struct {
			RoomID string `json:"room_id"`
		}
		if err := json.Unmarshal(pdu, &header); err != nil {
			util.GetLogger(t.context).WithError(err).Warn("Transaction: Failed to extract room ID from event")
			return nil, unmarshalError{err}
		}
		verReq := api.QueryRoomVersionForRoomRequest{RoomID: header.RoomID}
		verRes := api.QueryRoomVersionForRoomResponse{}
		if err := t.rsAPI.QueryRoomVersionForRoom(t.context, &verReq, &verRes); err != nil {
			util.GetLogger(t.context).WithError(err).Warn("Transaction: Failed to query room version for room", verReq.RoomID)
			return nil, roomNotFoundError{verReq.RoomID}
		}
		event, err := gomatrixserverlib.NewEventFromUntrustedJSON(pdu, verRes.RoomVersion)
		if err != nil {
			util.GetLogger(t.context).WithError(err).Warnf("Transaction: Failed to parse event JSON of event %q", event.EventID())
			return nil, unmarshalError{err}
		}
		if err := gomatrixserverlib.VerifyAllEventSignatures(t.context, []gomatrixserverlib.Event{event}, t.keys); err != nil {
			util.GetLogger(t.context).WithError(err).Warnf("Transaction: Couldn't validate signature of event %q", event.EventID())
			return nil, verifySigError{event.EventID(), err}
		}
		pdus = append(pdus, event.Headered(verRes.RoomVersion))
	}

	// Process the events.
	for _, e := range pdus {
		err := t.processEvent(e.Unwrap())
		if err != nil {
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
			switch err.(type) {
			case roomNotFoundError:
			case *gomatrixserverlib.NotAllowed:
			default:
				// Any other error should be the result of a temporary error in
				// our server so we should bail processing the transaction entirely.
				return nil, err
			}
			results[e.EventID()] = gomatrixserverlib.PDUResult{
				Error: err.Error(),
			}
			util.GetLogger(t.context).WithError(err).WithField("event_id", e.EventID()).Warn("Failed to process incoming federation event, skipping it.")
		} else {
			results[e.EventID()] = gomatrixserverlib.PDUResult{}
		}
	}

	t.processEDUs(t.EDUs)
	util.GetLogger(t.context).Infof("Processed %d PDUs from transaction %q", len(results), t.TransactionID)
	return &gomatrixserverlib.RespSend{PDUs: results}, nil
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

func (e roomNotFoundError) Error() string { return fmt.Sprintf("room %q not found", e.roomID) }
func (e unmarshalError) Error() string    { return fmt.Sprintf("unable to parse event: %s", e.err) }
func (e verifySigError) Error() string {
	return fmt.Sprintf("unable to verify signature of event %q: %s", e.eventID, e.err)
}

func (t *txnReq) processEDUs(edus []gomatrixserverlib.EDU) {
	for _, e := range edus {
		switch e.Type {
		case gomatrixserverlib.MTyping:
			// https://matrix.org/docs/spec/server_server/latest#typing-notifications
			var typingPayload struct {
				RoomID string `json:"room_id"`
				UserID string `json:"user_id"`
				Typing bool   `json:"typing"`
			}
			if err := json.Unmarshal(e.Content, &typingPayload); err != nil {
				util.GetLogger(t.context).WithError(err).Error("Failed to unmarshal typing event")
				continue
			}
			if err := t.eduProducer.SendTyping(t.context, typingPayload.UserID, typingPayload.RoomID, typingPayload.Typing, 30*1000); err != nil {
				util.GetLogger(t.context).WithError(err).Error("Failed to send typing event to edu server")
			}
		default:
			util.GetLogger(t.context).WithField("type", e.Type).Warn("unhandled edu")
		}
	}
}

func (t *txnReq) processEvent(e gomatrixserverlib.Event) error {
	prevEventIDs := e.PrevEventIDs()

	// Fetch the state needed to authenticate the event.
	needed := gomatrixserverlib.StateNeededForAuth([]gomatrixserverlib.Event{e})
	stateReq := api.QueryStateAfterEventsRequest{
		RoomID:       e.RoomID(),
		PrevEventIDs: prevEventIDs,
		StateToFetch: needed.Tuples(),
	}
	var stateResp api.QueryStateAfterEventsResponse
	if err := t.rsAPI.QueryStateAfterEvents(t.context, &stateReq, &stateResp); err != nil {
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
		return t.processEventWithMissingState(e, stateResp.RoomVersion)
	}

	// Check that the event is allowed by the state at the event.
	var events []gomatrixserverlib.Event
	for _, headeredEvent := range stateResp.StateEvents {
		events = append(events, headeredEvent.Unwrap())
	}
	if err := checkAllowedByState(e, events); err != nil {
		return err
	}

	// TODO: Check that the roomserver has a copy of all of the auth_events.
	// TODO: Check that the event is allowed by its auth_events.

	// pass the event to the roomserver
	_, err := t.producer.SendEvents(
		t.context,
		[]gomatrixserverlib.HeaderedEvent{
			e.Headered(stateResp.RoomVersion),
		},
		api.DoNotSendToOtherServers,
		nil,
	)
	return err
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

func (t *txnReq) processEventWithMissingState(e gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion) error {
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
	// TODO: Attempt to fill in the gap using /get_missing_events

	// Attempt to fetch the missing state using /state_ids and /events
	respState, haveEventIDs, err := t.lookupMissingStateViaStateIDs(e, roomVersion)
	if err != nil {
		// Fallback to /state
		util.GetLogger(t.context).WithError(err).Warn("processEventWithMissingState failed to /state_ids, falling back to /state")
		respState, err = t.lookupMissingStateViaState(e, roomVersion)
		if err != nil {
			return err
		}
	}

	// Check that the event is allowed by the state.
retryAllowedState:
	if err := checkAllowedByState(e, respState.StateEvents); err != nil {
		switch missing := err.(type) {
		case gomatrixserverlib.MissingAuthEventError:
			// An auth event was missing so let's look up that event over federation
			for _, s := range respState.StateEvents {
				if s.EventID() != missing.AuthEventID {
					continue
				}
				err = t.processEventWithMissingState(s, roomVersion)
				// If there was no error retrieving the event from federation then
				// we assume that it succeeded, so retry the original state check
				if err == nil {
					goto retryAllowedState
				}
			}
		default:
		}
		return err
	}

	// pass the event along with the state to the roomserver using a background context so we don't
	// needlessly expire
	return t.producer.SendEventWithState(context.Background(), respState, e.Headered(roomVersion), haveEventIDs)
}

func (t *txnReq) lookupMissingStateViaState(e gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion) (
	respState *gomatrixserverlib.RespState, err error) {
	state, err := t.federation.LookupState(t.context, t.Origin, e.RoomID(), e.EventID(), roomVersion)
	if err != nil {
		return nil, err
	}
	// Check that the returned state is valid.
	if err := state.Check(t.context, t.keys); err != nil {
		return nil, err
	}
	return &state, nil
}

func (t *txnReq) lookupMissingStateViaStateIDs(e gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion) (
	*gomatrixserverlib.RespState, map[string]bool, error) {

	// fetch the state event IDs at the time of the event
	stateIDs, err := t.federation.LookupStateIDs(t.context, t.Origin, e.RoomID(), e.EventID())
	if err != nil {
		return nil, nil, err
	}

	// fetch as many as we can from the roomserver, do them as 2 calls rather than
	// 1 to try to reduce the number of parameters in the bulk query this will use
	haveEventMap := make(map[string]*gomatrixserverlib.HeaderedEvent, len(stateIDs.StateEventIDs))
	haveEventIDs := make(map[string]bool)
	for _, eventList := range [][]string{stateIDs.StateEventIDs, stateIDs.AuthEventIDs} {
		queryReq := api.QueryEventsByIDRequest{
			EventIDs: eventList,
		}
		var queryRes api.QueryEventsByIDResponse
		if err = t.rsAPI.QueryEventsByID(t.context, &queryReq, &queryRes); err != nil {
			return nil, nil, err
		}
		// allow indexing of current state by event ID
		for i := range queryRes.Events {
			haveEventMap[queryRes.Events[i].EventID()] = &queryRes.Events[i]
			haveEventIDs[queryRes.Events[i].EventID()] = true
		}
	}

	// work out which auth/state IDs are missing
	wantIDs := append(stateIDs.StateEventIDs, stateIDs.AuthEventIDs...)
	missing := make(map[string]bool)
	for _, sid := range wantIDs {
		if _, ok := haveEventMap[sid]; !ok {
			missing[sid] = true
		}
	}
	util.GetLogger(t.context).WithFields(logrus.Fields{
		"missing":           len(missing),
		"event_id":          e.EventID(),
		"room_id":           e.RoomID(),
		"already_have":      len(haveEventMap),
		"total_state":       len(stateIDs.StateEventIDs),
		"total_auth_events": len(stateIDs.AuthEventIDs),
	}).Info("Fetching missing state at event")

	for missingEventID := range missing {
		var txn gomatrixserverlib.Transaction
		txn, err = t.federation.GetEvent(t.context, t.Origin, missingEventID)
		if err != nil {
			util.GetLogger(t.context).WithError(err).WithField("event_id", missingEventID).Warn("failed to get missing /event for event ID")
			return nil, nil, err
		}
		for _, pdu := range txn.PDUs {
			var event gomatrixserverlib.Event
			event, err = gomatrixserverlib.NewEventFromUntrustedJSON(pdu, roomVersion)
			if err != nil {
				util.GetLogger(t.context).WithError(err).Warnf("Transaction: Failed to parse event JSON of event %q", event.EventID())
				return nil, nil, unmarshalError{err}
			}
			if err = gomatrixserverlib.VerifyAllEventSignatures(t.context, []gomatrixserverlib.Event{event}, t.keys); err != nil {
				util.GetLogger(t.context).WithError(err).Warnf("Transaction: Couldn't validate signature of event %q", event.EventID())
				return nil, nil, verifySigError{event.EventID(), err}
			}
			h := event.Headered(roomVersion)
			haveEventMap[event.EventID()] = &h
		}
	}
	resp, err := t.createRespStateFromStateIDs(stateIDs, haveEventMap)
	return resp, haveEventIDs, err
}

func (t *txnReq) createRespStateFromStateIDs(stateIDs gomatrixserverlib.RespStateIDs, haveEventMap map[string]*gomatrixserverlib.HeaderedEvent) (
	*gomatrixserverlib.RespState, error) {
	// create a RespState response using the response to /state_ids as a guide
	respState := gomatrixserverlib.RespState{
		AuthEvents:  make([]gomatrixserverlib.Event, len(stateIDs.AuthEventIDs)),
		StateEvents: make([]gomatrixserverlib.Event, len(stateIDs.StateEventIDs)),
	}

	for i := range stateIDs.StateEventIDs {
		ev, ok := haveEventMap[stateIDs.StateEventIDs[i]]
		if !ok {
			return nil, fmt.Errorf("missing state event %s", stateIDs.StateEventIDs[i])
		}
		respState.StateEvents[i] = ev.Unwrap()
	}
	for i := range stateIDs.AuthEventIDs {
		ev, ok := haveEventMap[stateIDs.AuthEventIDs[i]]
		if !ok {
			return nil, fmt.Errorf("missing auth event %s", stateIDs.AuthEventIDs[i])
		}
		respState.AuthEvents[i] = ev.Unwrap()
	}
	// Check that the returned state is valid.
	if err := respState.Check(t.context, t.keys); err != nil {
		return nil, err
	}
	return &respState, nil
}
