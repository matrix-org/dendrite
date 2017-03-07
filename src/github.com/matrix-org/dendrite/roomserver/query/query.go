package query

import (
	"encoding/json"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
)

// RoomserverQueryAPIDatabase has the storage APIs needed to implement the query API.
type RoomserverQueryAPIDatabase interface {
	// Lookup the numeric ID for the room.
	// Returns 0 if the room doesn't exists.
	// Returns an error if there was a problem talking to the database.
	RoomNID(roomID string) (types.RoomNID, error)
	// Lookup event references for the latest events in the room and the current state snapshot.
	// Returns an error if there was a problem talking to the database.
	LatestEventIDs(roomNID types.RoomNID) ([]gomatrixserverlib.EventReference, types.StateSnapshotNID, error)
	// Lookup the numeric IDs for a list of string event types.
	// Returns a map from string event type to numeric ID for the event type.
	EventTypeNIDs(eventTypes []string) (map[string]types.EventTypeNID, error)
	// Lookup the numeric IDs for a list of string event state keys.
	// Returns a map from string state key to numeric ID for the state key.
	EventStateKeyNIDs(eventStateKeys []string) (map[string]types.EventStateKeyNID, error)
	// Lookup the numeric state block IDs for each numeric state snapshot ID
	// The returned slice is sorted by numeric state snapshot ID.
	StateBlockNIDs(stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error)
	// Lookup the state data for the state key tuples for each numeric state block ID
	// The returned slice is sorted by numeric state block ID.
	StateEntriesForTuples(stateBlockNIDs []types.StateBlockNID, stateKeyTuples []types.StateKeyTuple) (
		[]types.StateEntryList, error,
	)
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
	stateTuples, err := stringTuplesToNumericTuples(r.DB, request.StateToFetch)
	if err != nil {
		return err
	}

	stateEntries, err := loadStateAtSnapshotForTuples(r.DB, currentStateSnapshotNID, stateTuples)
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
