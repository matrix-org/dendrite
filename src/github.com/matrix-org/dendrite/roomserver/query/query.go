package query

import (
	"encoding/json"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
)

// RoomserverQueryAPIDatabase has the storage APIs needed to implement the query API.
type RoomserverQueryAPIDatabase interface {
	state.RoomStateDatabase
	// Lookup the numeric ID for the room.
	// Returns 0 if the room doesn't exists.
	// Returns an error if there was a problem talking to the database.
	RoomNID(roomID string) (types.RoomNID, error)
	// Lookup event references for the latest events in the room and the current state snapshot.
	// Returns an error if there was a problem talking to the database.
	LatestEventIDs(roomNID types.RoomNID) ([]gomatrixserverlib.EventReference, types.StateSnapshotNID, error)
	// Lookup the Events for a list of numeric event IDs.
	// Returns a sorted list of events.
	Events(eventNIDs []types.EventNID) ([]types.Event, error)
}

// RoomserverQueryAPI is an implementation of RoomserverQueryAPI
type RoomserverQueryAPI struct {
	DB RoomserverQueryAPIDatabase
}

// QueryLatestEventsAndState implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryLatestEventsAndState(
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) (err error) {
	response.QueryLatestEventsAndStateRequest = *request
	roomNID, err := r.DB.RoomNID(request.RoomID)
	if err != nil {
		return err
	}
	if roomNID == 0 {
		return nil
	}
	response.RoomExists = true
	var currentStateSnapshotNID types.StateSnapshotNID
	response.LatestEvents, currentStateSnapshotNID, err = r.DB.LatestEventIDs(roomNID)
	if err != nil {
		return err
	}

	// Lookup the currrent state for the requested tuples.
	stateEntries, err := state.LoadStateAtSnapshotForStringTuples(r.DB, currentStateSnapshotNID, request.StateToFetch)
	if err != nil {
		return err
	}

	eventNIDs := make([]types.EventNID, len(stateEntries))
	for i := range stateEntries {
		eventNIDs[i] = stateEntries[i].EventNID
	}

	stateEvents, err := r.DB.Events(eventNIDs)
	if err != nil {
		return err
	}

	response.StateEvents = make([]gomatrixserverlib.Event, len(stateEvents))
	for i := range stateEvents {
		response.StateEvents[i] = stateEvents[i].Event
	}
	return nil
}

// SetupHTTP adds the RoomserverQueryAPI handlers to the http.ServeMux.
func (r *RoomserverQueryAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(
		api.RoomserverQueryLatestEventsAndStatePath,
		makeAPI("query_latest_events_and_state", func(req *http.Request) util.JSONResponse {
			var request api.QueryLatestEventsAndStateRequest
			var response api.QueryLatestEventsAndStateResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryLatestEventsAndState(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
}

func makeAPI(metric string, apiFunc func(req *http.Request) util.JSONResponse) http.Handler {
	return prometheus.InstrumentHandler(metric, util.MakeJSONAPI(util.NewJSONRequestHandler(apiFunc)))
}
