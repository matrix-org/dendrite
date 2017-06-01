package writers

import (
	"encoding/json"
	"fmt"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
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

	// TODO: process the transaction.

	return util.JSONResponse{
		Code: 200,
		JSON: gomatrixserverlib.RespSend{},
	}
}

func processTransaction(t gomatrixserverlib.Transaction, query api.RoomserverQueryAPI) {

}

var errUnknownRoom = fmt.Errorf("unknown room")

func processEvent(e gomatrixserverlib.Event, query api.RoomserverQueryAPI) error {
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
		return errUnknownRoom
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

	authUsingState := gomatrixserverlib.NewAuthEvents(nil)
	for i := range stateResp.StateEvents {
		authUsingState.AddEvent(&stateResp.StateEvents[i])
	}
	err := gomatrixserverlib.Allowed(e, &authUsingState)
	if err != nil {
		return err
	}

	return nil
}
