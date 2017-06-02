package writers

import (
	"encoding/json"
	"fmt"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/federationapi/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"net/http"
	"time"
)

// Send implements /_matrix/federation/v1/send/{txnID}
func Send(
	req *http.Request,
	txnID gomatrixserverlib.TransactionID,
	now time.Time,
	cfg config.FederationAPI,
	query api.RoomserverQueryAPI,
	producer *producers.RoomserverProducer,
	keys gomatrixserverlib.KeyRing,
) util.JSONResponse {
	request, errResp := gomatrixserverlib.VerifyHTTPRequest(req, now, cfg.ServerName, keys)
	if request == nil {
		return errResp
	}

	var content gomatrixserverlib.Transaction
	if err := json.Unmarshal(request.Content(), &content); err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}

	content.Origin = request.Origin()
	content.TransactionID = txnID
	content.Destination = cfg.ServerName

	resp, err := processTransaction(content, query, producer, keys)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: resp,
	}
}

func processTransaction(
	t gomatrixserverlib.Transaction,
	query api.RoomserverQueryAPI,
	producer *producers.RoomserverProducer,
	keys gomatrixserverlib.KeyRing,
) (*gomatrixserverlib.RespSend, error) {
	// Check the event signatures
	if err := gomatrixserverlib.VerifyEventSignatures(t.PDUs, keys); err != nil {
		return nil, err
	}

	// Process the events.
	results := map[string]gomatrixserverlib.PDUResult{}
	for _, e := range t.PDUs {
		err := processEvent(e, query, producer)
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
			results[e.EventID()] = gomatrixserverlib.PDUResult{err.Error()}
		} else {
			results[e.EventID()] = gomatrixserverlib.PDUResult{}
		}
	}

	// TODO: Process the EDUs.

	return &gomatrixserverlib.RespSend{PDUs: results}, nil
}

type unknownRoomError string

func (e unknownRoomError) Error() string { return fmt.Sprintf("unknown room %q", e) }

func processEvent(
	e gomatrixserverlib.Event,
	query api.RoomserverQueryAPI,
	producer *producers.RoomserverProducer,
) error {
	refs := e.PrevEvents()
	prevEventIDs := make([]string, len(refs))
	for i := range refs {
		prevEventIDs[i] = refs[i].EventID
	}

	// Fetch the state needed to authenticate the event.
	needed := gomatrixserverlib.StateNeededForAuth([]gomatrixserverlib.Event{e})
	stateReq := api.QueryStateAfterEventsRequest{
		RoomID:       e.RoomID(),
		PrevEventIDs: prevEventIDs,
		StateToFetch: needed.Tuples(),
	}
	var stateResp api.QueryStateAfterEventsResponse
	if err := query.QueryStateAfterEvents(&stateReq, &stateResp); err != nil {
		return err
	}

	if !stateResp.RoomExists {
		// TODO: When synapse receives a message for a room it is not in it
		// asked the remote server for the state of the room so that it can
		// check if the remote server knows of a join "m.room.member" event
		// that this server is unaware of.
		// However generally speaking we should reject events for rooms we
		// aren't a member of.
		return unknownRoomError(e.RoomID())
	}

	if !stateResp.PrevEventsExist {
		// We are missing the previous events for this events.
		// This means that there is a gap in our view of the history of the
		// room. There two ways that we can handle such a gap:
		//   1) We can fill in the gap using /get_missing_events
		//   2) We can leave the gap and request the state of the room at
		//      this event from the remote server using either /state_ids
		//      or /state.
		// Synapse will attempt to do 1 and if that fails or if the gap is
		// too large then it will attempt 2.
		// Synapse will use /state_ids if possible since ususally the state
		// is largely unchanged and it is more efficient to fetch a list of
		// event ids and then use /event to fetch the individual events.
		// However not all version of synapse support /state_ids so you may
		// need to fallback to /state.
		// TODO: Attempt to fill in the gap using /get_missing_events
		// TODO: Attempt to fetch the state using /state_ids and /events
		// TODO: Attempt to fetch the state using /state
		panic(fmt.Errorf("Receiving events with missing prev_events is no implemented"))
	}

	// Check that the event is allowed by the state at the event.
	authUsingState := gomatrixserverlib.NewAuthEvents(nil)
	for i := range stateResp.StateEvents {
		authUsingState.AddEvent(&stateResp.StateEvents[i])
	}
	err := gomatrixserverlib.Allowed(e, &authUsingState)
	if err != nil {
		return err
	}

	// TODO: Check that the roomserver has a copy of all of the auth_events.
	// TODO: Check that the event is allowed by its auth_events.

	// pass the event to the roomserver
	if err := producer.SendEvents([]gomatrixserverlib.Event{e}); err != nil {
		return err
	}

	return nil
}
