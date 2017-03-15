package writers

import (
	"net/http"

	"fmt"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// http://matrix.org/docs/spec/client_server/r0.2.0.html#put-matrix-client-r0-rooms-roomid-send-eventtype-txnid
// http://matrix.org/docs/spec/client_server/r0.2.0.html#put-matrix-client-r0-rooms-roomid-state-eventtype-statekey
type sendEventResponse struct {
	EventID string `json:"event_id"`
}

// SendEvent implements:
//   /rooms/{roomID}/send/{eventType}/{txnID}
//   /rooms/{roomID}/state/{eventType}/{stateKey}
func SendEvent(req *http.Request, roomID, eventType, txnID string, stateKey *string, cfg config.ClientAPI, queryAPI api.RoomserverQueryAPI, producer *producers.RoomserverProducer) util.JSONResponse {
	// parse the incoming http request
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return *resErr
	}
	var r map[string]interface{} // must be a JSON object
	resErr = httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	// create the new event and set all the fields we can
	builder := gomatrixserverlib.EventBuilder{
		Sender:   userID,
		RoomID:   roomID,
		Type:     eventType,
		StateKey: stateKey,
	}
	builder.SetContent(r)

	// work out what will be required in order to send this event
	requiredStateEvents, err := stateNeeded(&builder)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	// Ask the roomserver for information about this room
	queryReq := api.QueryLatestEventsAndStateRequest{
		RoomID:       roomID,
		StateToFetch: requiredStateEvents,
	}
	var queryRes api.QueryLatestEventsAndStateResponse
	if queryErr := queryAPI.QueryLatestEventsAndState(&queryReq, &queryRes); queryErr != nil {
		return httputil.LogThenError(req, queryErr)
	}
	if !queryRes.RoomExists {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	}

	// set the fields we previously couldn't do and build the event
	builder.PrevEvents = queryRes.LatestEvents // the current events will be the prev events of the new event
	var refs []gomatrixserverlib.EventReference
	for _, e := range queryRes.StateEvents {
		refs = append(refs, e.EventReference())
	}
	builder.AuthEvents = refs
	eventID := fmt.Sprintf("$%s:%s", util.RandomString(16), cfg.ServerName)
	e, err := builder.Build(eventID, time.Now(), cfg.ServerName, cfg.KeyID, cfg.PrivateKey)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	// check to see if this user can perform this operation
	stateEvents := make([]*gomatrixserverlib.Event, len(queryRes.StateEvents))
	for i := range queryRes.StateEvents {
		stateEvents[i] = &queryRes.StateEvents[i]
	}
	provider := gomatrixserverlib.NewAuthEvents(stateEvents)
	if err = gomatrixserverlib.Allowed(e, &provider); err != nil {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden(err.Error()), // TODO: Is this error string comprehensible to the client?
		}
	}

	// pass the new event to the roomserver
	if err := producer.SendEvents([]gomatrixserverlib.Event{e}); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: sendEventResponse{e.EventID()},
	}
}

func stateNeeded(builder *gomatrixserverlib.EventBuilder) (requiredStateEvents []common.StateKeyTuple, err error) {
	authEvents, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		return
	}
	if authEvents.Create {
		requiredStateEvents = append(requiredStateEvents, common.StateKeyTuple{"m.room.create", ""})
	}
	if authEvents.JoinRules {
		requiredStateEvents = append(requiredStateEvents, common.StateKeyTuple{"m.room.join_rules", ""})
	}
	if authEvents.PowerLevels {
		requiredStateEvents = append(requiredStateEvents, common.StateKeyTuple{"m.room.power_levels", ""})
	}
	for _, userID := range authEvents.Member {
		requiredStateEvents = append(requiredStateEvents, common.StateKeyTuple{"m.room.member", userID})
	}
	for _, token := range authEvents.ThirdPartyInvite {
		requiredStateEvents = append(requiredStateEvents, common.StateKeyTuple{"m.room.third_party_invite", token})
	}
	return
}
