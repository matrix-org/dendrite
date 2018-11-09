// Copyright 2018 New Vector Ltd
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
	"net/http"
	"strconv"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

type messageResp struct {
	Start string                          `json:"start"`
	End   string                          `json:"end"`
	Chunk []gomatrixserverlib.ClientEvent `json:"chunk"`
}

const defaultMessagesLimit = 10

// OnIncomingMessagesRequest implements the /messages endpoint from the
// client-server API.
// See: https://matrix.org/docs/spec/client_server/r0.4.0.html#get-matrix-client-r0-rooms-roomid-messages
func OnIncomingMessagesRequest(
	req *http.Request, db *storage.SyncServerDatabase, roomID string,
	federation *gomatrixserverlib.FederationClient,
	queryAPI api.RoomserverQueryAPI,
	cfg *config.Dendrite,
) util.JSONResponse {
	var from, to int
	var err error

	// Extract parameters from the request's URL.
	// Pagination tokens.
	from, err = strconv.Atoi(req.URL.Query().Get("from"))
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("from could not be parsed into an integer: " + err.Error()),
		}
	}
	fromPos := types.StreamPosition(from)

	// Direction to return events from.
	dir := req.URL.Query().Get("dir")
	if dir != "b" && dir != "f" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Bad or missing dir query parameter (should be either 'b' or 'f')"),
		}
	}
	// A boolean is easier to handle in this case, especially since dir is sure
	// to have one of the two accepted values (so dir == "f" <=> !backwardOrdering).
	backwardOrdering := (dir == "b")

	// Pagination tokens. To is optional, and its default value depends on the
	// direction ("b" or "f").
	toStr := req.URL.Query().Get("to")
	var toPos types.StreamPosition
	if len(toStr) > 0 {
		to, err = strconv.Atoi(toStr)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidArgumentValue("to could not be parsed into an integer: " + err.Error()),
			}
		}
		toPos = types.StreamPosition(to)
	} else {
		// If "to" isn't provided, it defaults to either the earliest stream
		// position (if we're going backward) or to the latest one (if we're
		// going forward).
		toPos, err = setToDefault(req.Context(), backwardOrdering, db)
		if err != nil {
			return httputil.LogThenError(req, err)
		}
	}

	// Maximum number of events to return; defaults to 10.
	limit := defaultMessagesLimit
	if len(req.URL.Query().Get("limit")) > 0 {
		limit, err = strconv.Atoi(req.URL.Query().Get("limit"))

		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidArgumentValue("limit could not be parsed into an integer: " + err.Error()),
			}
		}
	}
	// TODO: Implement filtering (#587)

	// Check the room ID's format.
	if _, _, err = gomatrixserverlib.SplitID('!', roomID); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Bad room ID: " + err.Error()),
		}
	}

	clientEvents, start, end, err := retrieveEvents(
		req.Context(), db, roomID, fromPos, toPos, toStr, limit, backwardOrdering,
		federation, queryAPI, cfg,
	)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	// Respond with the events.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: messageResp{
			Chunk: clientEvents,
			Start: start,
			End:   end,
		},
	}
}

// setToDefault returns the default value for the "to" query parameter of a
// request to /messages if not provided. It defaults to either the earliest
// stream position (if we're going backward) or to the latest one (if we're
// going forward).
// Returns an error if there was an issue with retrieving the latest position
// from the database
func setToDefault(
	ctx context.Context, backwardOrdering bool, db *storage.SyncServerDatabase,
) (toPos types.StreamPosition, err error) {
	if backwardOrdering {
		toPos = types.StreamPosition(1)
	} else {
		toPos, err = db.SyncStreamPosition(ctx)
		if err != nil {
			return
		}
	}

	return
}

// retrieveEvents retrieve events from the local database for a request on
// /messages. If there's not enough events to retrieve, it asks another
// homeserver in the room for older events.
// Returns an error if there was an issue talking to the database or with the
// remote homeserver.
func retrieveEvents(
	ctx context.Context, db *storage.SyncServerDatabase, roomID string,
	fromPos, toPos types.StreamPosition, toStr string, limit int,
	backwardOrdering bool,
	federation *gomatrixserverlib.FederationClient,
	queryAPI api.RoomserverQueryAPI,
	cfg *config.Dendrite,
) (clientEvents []gomatrixserverlib.ClientEvent, start string, end string, err error) {
	// Retrieve the events from the local database.
	streamEvents, err := db.GetEventsInRange(
		ctx, fromPos, toPos, roomID, limit, backwardOrdering,
	)
	if err != nil {
		return
	}

	// Check if we have enough events.
	isSetLargeEnough := true
	if len(streamEvents) < limit {
		if backwardOrdering {
			if len(toStr) > 0 {
				// The condition in the SQL query is a strict "greater than" so
				// we need to check against to-1.
				isSetLargeEnough = (toPos-1 == streamEvents[0].StreamPosition)
			}
		} else {
			isSetLargeEnough = (fromPos-1 == streamEvents[0].StreamPosition)
		}
	}

	// Check if earliest event is a backward extremity, i.e. if one of its
	// previous events is missing from the database.
	// Get the earliest retrieved event's parents.

	backwardExtremity, err := isBackwardExtremity(ctx, &(streamEvents[0]), db)
	if err != nil {
		return
	}

	var events []gomatrixserverlib.Event

	// Backfill is needed if we've reached a backward extremity and need more
	// events. It's only needed if the direction is backard.
	if backwardExtremity && !isSetLargeEnough && backwardOrdering {
		var pdus []gomatrixserverlib.Event
		// Only ask the remote server for enough events to reach the limit.
		pdus, err = backfill(
			ctx, roomID, streamEvents[0].EventID(), limit-len(streamEvents),
			queryAPI, cfg, federation,
		)
		if err != nil {
			return
		}

		events = append(events, pdus...)
	}

	// Append the events we retrieved locally, then convert them into client
	// events.
	events = append(events, storage.StreamEventsToEvents(nil, streamEvents)...)
	clientEvents = gomatrixserverlib.ToClientEvents(events, gomatrixserverlib.FormatAll)
	start = streamEvents[0].StreamPosition.String()
	end = streamEvents[len(streamEvents)-1].StreamPosition.String()

	return
}

// isBackwardExtremity checks if a given event is a backward extremity. It does
// so by checking the presence in the database of all of its parent events, and
// consider the event itself a backward extremity if at least one of the parent
// events doesn't exist in the database.
// Returns an error if there was an issue with talking to the database.
func isBackwardExtremity(
	ctx context.Context, ev *storage.StreamEvent, db *storage.SyncServerDatabase,
) (bool, error) {
	prevIDs := ev.PrevEventIDs()
	prevs, err := db.Events(ctx, prevIDs)
	if err != nil {
		return false, nil
	}
	// Check if we have all of the events we requested. If not, it means we've
	// reached a backard extremity.
	var eventInDB bool
	var id string
	// Iterate over the IDs we used in the request.
	for _, id = range prevIDs {
		eventInDB = false
		// Iterate over the events we got in response.
		for _, ev := range prevs {
			if ev.EventID() == id {
				eventInDB = true
			}
		}
		// One occurrence of one the event's parents not being present in the
		// database is enough to say that the event is a backward extremity.
		if !eventInDB {
			return true, nil
		}
	}

	return false, nil
}

// backfill performs a backfill request over the federation on another
// homeserver in the room.
// See: https://matrix.org/docs/spec/server_server/unstable.html#get-matrix-federation-v1-backfill-roomid
// Returns with an empty string if the remote homeserver didn't return with any
// event, or if there is no remote homeserver to contact.
// Returns an error if there was an issue with retrieving the list of servers in
// the room or sending the request.
func backfill(
	ctx context.Context, roomID, fromEventID string, limit int,
	queryAPI api.RoomserverQueryAPI, cfg *config.Dendrite,
	federation *gomatrixserverlib.FederationClient,
) ([]gomatrixserverlib.Event, error) {
	// Query the list of servers in the room when the earlier event we know
	// of was sent.
	var serversResponse api.QueryServersInRoomAtEventResponse
	serversRequest := api.QueryServersInRoomAtEventRequest{
		RoomID:  roomID,
		EventID: fromEventID,
	}
	if err := queryAPI.QueryServersInRoomAtEvent(ctx, &serversRequest, &serversResponse); err != nil {
		return nil, err
	}

	// Use the first server from the response, except if that server is us.
	// In that case, use the second one if the roomserver responded with
	// enough servers. If not, use an empty string to prevent the backfill
	// from happening as there's no server to direct the request towards.
	// TODO: Be smarter at selecting the server to direct the request
	// towards.
	srvToBackfillFrom := serversResponse.Servers[0]
	if srvToBackfillFrom == cfg.Matrix.ServerName {
		if len(serversResponse.Servers) > 1 {
			srvToBackfillFrom = serversResponse.Servers[1]
		} else {
			srvToBackfillFrom = gomatrixserverlib.ServerName("")
			log.Warn("Not enough servers to backfill from")
		}
	}

	pdus := make([]gomatrixserverlib.Event, 0)

	// If the roomserver responded with at least one server that isn't us,
	// send it a request for backfill.
	if len(srvToBackfillFrom) > 0 {
		txn, err := federation.Backfill(
			ctx, srvToBackfillFrom, roomID, limit, []string{fromEventID},
		)
		if err != nil {
			return nil, err
		}

		// TODO: Store the events in the database. The remaining question to
		// make this possible is what to assign to the new events' stream
		// position (negative integers? change the stream position format into a
		// timestamp-based one?...)
		pdus = txn.PDUs
	}

	return pdus, nil
}
