package writers

import (
	"encoding/json"
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

func processEvent(e gomatrixserverlib.Event, query api.RoomserverQueryAPI) error {
	refs := e.PrevEvents()
	prevEventIDs := make([]string, len(refs))
	for i := range refs {
		prevEventIDs[i] = refs[i].EventID
	}

	needed := gomatrixserverlib.StateNeededForAuth([]gomatrixserverlib.Event{e})

	request := api.QueryStateAfterEventsRequest{
		RoomID:       e.RoomID(),
		PrevEventIDs: prevEventIDs,
		StateToFetch: needed.Tuples(),
	}
	var response api.QueryStateAfterEventsResponse
	if err := query.QueryStateAfterEvents(&request, &response); err != nil {
		return err
	}

	// TODO process the event.

	return nil
}
