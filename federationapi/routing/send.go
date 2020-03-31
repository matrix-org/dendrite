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
)

// Send implements /_matrix/federation/v1/send/{txnID}
func Send(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	txnID gomatrixserverlib.TransactionID,
	cfg *config.Dendrite,
	query api.RoomserverQueryAPI,
	producer *producers.RoomserverProducer,
	keys gomatrixserverlib.KeyRing,
	federation *gomatrixserverlib.FederationClient,
) util.JSONResponse {
	t := txnReq{
		context:    httpReq.Context(),
		query:      query,
		producer:   producer,
		keys:       keys,
		federation: federation,
	}

	var txnEvents struct {
		PDUs []json.RawMessage `json:"pdus"`
		EDUs []json.RawMessage `json:"edus"`
	}

	if err := json.Unmarshal(request.Content(), &txnEvents); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}

	t.PDUs = txnEvents.PDUs
	t.Origin = request.Origin()
	t.TransactionID = txnID
	t.Destination = cfg.Matrix.ServerName

	util.GetLogger(httpReq.Context()).Infof("Received transaction %q containing %d PDUs, %d EDUs", txnID, len(t.PDUs), len(t.EDUs))

	resp, err := t.processTransaction()
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("t.processTransaction failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}

type txnReq struct {
	gomatrixserverlib.Transaction
	context    context.Context
	query      api.RoomserverQueryAPI
	producer   *producers.RoomserverProducer
	keys       gomatrixserverlib.KeyRing
	federation *gomatrixserverlib.FederationClient
}

func (t *txnReq) processTransaction() (*gomatrixserverlib.RespSend, error) {
	var pdus []gomatrixserverlib.HeaderedEvent
	for _, pdu := range t.PDUs {
		var header struct {
			RoomID string `json:"room_id"`
		}
		if err := json.Unmarshal(pdu, &header); err != nil {
			util.GetLogger(t.context).WithError(err).Warn("Transaction: Failed to extract room ID from event")
			return nil, err
		}
		verReq := api.QueryRoomVersionForRoomRequest{RoomID: header.RoomID}
		verRes := api.QueryRoomVersionForRoomResponse{}
		if err := t.query.QueryRoomVersionForRoom(t.context, &verReq, &verRes); err != nil {
			util.GetLogger(t.context).WithError(err).Warn("Transaction: Failed to query room version for room", verReq.RoomID)
			return nil, err
		}
		event, err := gomatrixserverlib.NewEventFromUntrustedJSON(pdu, verRes.RoomVersion)
		if err != nil {
			util.GetLogger(t.context).WithError(err).Warnf("Transaction: Failed to parse event JSON of event %q", event.EventID())
			return nil, err
		}
		if err := gomatrixserverlib.VerifyAllEventSignatures(t.context, []*gomatrixserverlib.Event{event}, t.keys); err != nil {
			util.GetLogger(t.context).WithError(err).Warnf("Transaction: Couldn't validate signature of event %q", event.EventID())
			return nil, err
		}
		pdus = append(pdus, event.Headered(verRes.RoomVersion))
	}

	// Process the events.
	results := map[string]gomatrixserverlib.PDUResult{}
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
			case unknownRoomError:
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

	// TODO: Process the EDUs.
	util.GetLogger(t.context).Infof("Processed %d PDUs from transaction %q", len(results), t.TransactionID)
	return &gomatrixserverlib.RespSend{PDUs: results}, nil
}

type unknownRoomError struct {
	roomID string
}

func (e unknownRoomError) Error() string { return fmt.Sprintf("unknown room %q", e.roomID) }

func (t *txnReq) processEvent(e *gomatrixserverlib.Event) error {
	prevEventIDs := e.PrevEventIDs()

	// Fetch the state needed to authenticate the event.
	needed := gomatrixserverlib.StateNeededForAuth([]*gomatrixserverlib.Event{e})
	stateReq := api.QueryStateAfterEventsRequest{
		RoomID:       e.RoomID(),
		PrevEventIDs: prevEventIDs,
		StateToFetch: needed.Tuples(),
	}
	var stateResp api.QueryStateAfterEventsResponse
	if err := t.query.QueryStateAfterEvents(t.context, &stateReq, &stateResp); err != nil {
		return err
	}

	if !stateResp.RoomExists {
		// TODO: When synapse receives a message for a room it is not in it
		// asks the remote server for the state of the room so that it can
		// check if the remote server knows of a join "m.room.member" event
		// that this server is unaware of.
		// However generally speaking we should reject events for rooms we
		// aren't a member of.
		return unknownRoomError{e.RoomID()}
	}

	if !stateResp.PrevEventsExist {
		return t.processEventWithMissingState(e, stateResp.RoomVersion)
	}

	// Check that the event is allowed by the state at the event.
	var events []*gomatrixserverlib.Event
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

func (t *txnReq) processEventWithMissingState(e *gomatrixserverlib.Event, roomVersion gomatrixserverlib.RoomVersion) error {
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
	// TODO: Attempt to fetch the state using /state_ids and /events
	state, err := t.federation.LookupState(t.context, t.Origin, e.RoomID(), e.EventID(), roomVersion)
	if err != nil {
		return err
	}
	// Check that the returned state is valid.
	if err := state.Check(t.context, t.keys); err != nil {
		return err
	}
	// Check that the event is allowed by the state.
retryAllowedState:
	if err := checkAllowedByState(e, state.StateEvents); err != nil {
		switch missing := err.(type) {
		case gomatrixserverlib.MissingAuthEventError:
			// An auth event was missing so let's look up that event over federation
			for _, s := range state.StateEvents {
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

	// pass the event along with the state to the roomserver
	return t.producer.SendEventWithState(t.context, state, e.Headered(roomVersion))
}
