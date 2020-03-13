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
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
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
	fmt.Println("TRANSACTION CONTENT IS", string(request.Content()))

	t := txnReq{
		context:    httpReq.Context(),
		query:      query,
		producer:   producer,
		keys:       keys,
		federation: federation,
	}

	// we need to work out what the room version is for each of these events
	// or otherwise we will not be able to unmarshal them from the tx
	pdus := gjson.GetBytes(request.Content(), "pdus")
	if rest, err := sjson.DeleteBytes(request.Content(), "pdus"); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("Extracting PDUs from JSON failed. " + err.Error()),
		}
	} else if err := json.Unmarshal(rest, &t); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}

	fmt.Println("Number of PDUs:", len(t.PDUs))
	fmt.Println("Number of EDUs:", len(t.EDUs))

	roomVersions := make(map[string]gomatrixserverlib.RoomVersion)
	if pdus.Exists() {
		for _, pdu := range pdus.Array() {
			if rid := pdu.Get("room_id"); rid.Exists() {
				// Look up the room version for this room if we don't already
				// know it.
				if _, ok := roomVersions[rid.String()]; !ok {
					// Look up the room version for this room.
					vReq := api.QueryRoomVersionForRoomIDRequest{RoomID: rid.String()}
					vRes := api.QueryRoomVersionForRoomIDResponse{}
					if query.QueryRoomVersionForRoomID(httpReq.Context(), &vReq, &vRes) == nil {
						roomVersions[rid.String()] = vRes.RoomVersion
						fmt.Println("Room version for", rid.String(), "is", vRes.RoomVersion)
					}
				}

				// Check if we know the room ID again. It's possible the previous
				// step failed.
				if roomVersion, ok := roomVersions[rid.String()]; ok {
					fmt.Println("We know the room version for", rid.String())
					// Now unmarshal the event.
					event, err := gomatrixserverlib.NewEventFromUntrustedJSON([]byte(pdu.Raw), roomVersion)
					if err != nil {
						fmt.Println("Couldn't create event:", err)
						continue
					}

					fmt.Println("Event", event.EventID(), "was created successfully")
					fmt.Println("Auth events:", event.AuthEventIDs())
					fmt.Println("Prev events:", event.PrevEventIDs())

					// Assuming nothing has gone wrong at this point, we can now
					// append our newly formatted event into the transaction object.
					t.PDUs = append(t.PDUs, event)
				} else {
					fmt.Println("We don't know the room version for", rid.String())
				}
			}
		}
	}

	fmt.Println("Number of PDUs:", len(t.PDUs))
	fmt.Println("Number of EDUs:", len(t.EDUs))

	t.Origin = request.Origin()
	t.TransactionID = txnID
	t.Destination = cfg.Matrix.ServerName

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
	// Check the event signatures
	if err := gomatrixserverlib.VerifyAllEventSignatures(t.context, t.PDUs, t.keys); err != nil {
		return nil, err
	}

	// Process the events.
	results := map[string]gomatrixserverlib.PDUResult{}
	for _, e := range t.PDUs {
		err := t.processEvent(e)
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
		} else {
			results[e.EventID()] = gomatrixserverlib.PDUResult{}
		}
	}

	// TODO: Process the EDUs.

	return &gomatrixserverlib.RespSend{PDUs: results}, nil
}

type unknownRoomError struct {
	roomID string
}

func (e unknownRoomError) Error() string { return fmt.Sprintf("unknown room %q", e.roomID) }

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
		return t.processEventWithMissingState(e)
	}

	// Check that the event is allowed by the state at the event.
	if err := checkAllowedByState(e, stateResp.StateEvents); err != nil {
		return err
	}

	// TODO: Check that the roomserver has a copy of all of the auth_events.
	// TODO: Check that the event is allowed by its auth_events.

	// pass the event to the roomserver
	_, err := t.producer.SendEvents(t.context, []gomatrixserverlib.Event{e}, api.DoNotSendToOtherServers, nil)
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

func (t *txnReq) processEventWithMissingState(e gomatrixserverlib.Event) error {
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
	state, err := t.federation.LookupState(t.context, t.Origin, e.RoomID(), e.EventID())
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
				err = t.processEventWithMissingState(s)
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
	return t.producer.SendEventWithState(t.context, state, e)
}
