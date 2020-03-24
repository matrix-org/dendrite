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
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type messagesReq struct {
	ctx              context.Context
	db               storage.Database
	queryAPI         api.RoomserverQueryAPI
	federation       *gomatrixserverlib.FederationClient
	cfg              *config.Dendrite
	roomID           string
	from             *types.PaginationToken
	to               *types.PaginationToken
	wasToProvided    bool
	limit            int
	backwardOrdering bool
}

type messagesResp struct {
	Start string                          `json:"start"`
	End   string                          `json:"end"`
	Chunk []gomatrixserverlib.ClientEvent `json:"chunk"`
}

const defaultMessagesLimit = 10

// OnIncomingMessagesRequest implements the /messages endpoint from the
// client-server API.
// See: https://matrix.org/docs/spec/client_server/latest.html#get-matrix-client-r0-rooms-roomid-messages
func OnIncomingMessagesRequest(
	req *http.Request, db storage.Database, roomID string,
	federation *gomatrixserverlib.FederationClient,
	queryAPI api.RoomserverQueryAPI,
	cfg *config.Dendrite,
) util.JSONResponse {
	var err error

	// Extract parameters from the request's URL.
	// Pagination tokens.
	from, err := types.NewPaginationTokenFromString(req.URL.Query().Get("from"))
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("Invalid from parameter: " + err.Error()),
		}
	}

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
	var to *types.PaginationToken
	wasToProvided := true
	if s := req.URL.Query().Get("to"); len(s) > 0 {
		to, err = types.NewPaginationTokenFromString(s)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidArgumentValue("Invalid to parameter: " + err.Error()),
			}
		}
	} else {
		// If "to" isn't provided, it defaults to either the earliest stream
		// position (if we're going backward) or to the latest one (if we're
		// going forward).
		to, err = setToDefault(req.Context(), db, backwardOrdering, roomID)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("setToDefault failed")
			return jsonerror.InternalServerError()
		}
		wasToProvided = false
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

	mReq := messagesReq{
		ctx:              req.Context(),
		db:               db,
		queryAPI:         queryAPI,
		federation:       federation,
		cfg:              cfg,
		roomID:           roomID,
		from:             from,
		to:               to,
		wasToProvided:    wasToProvided,
		limit:            limit,
		backwardOrdering: backwardOrdering,
	}

	clientEvents, start, end, err := mReq.retrieveEvents()
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("mreq.retrieveEvents failed")
		return jsonerror.InternalServerError()
	}
	util.GetLogger(req.Context()).WithFields(logrus.Fields{
		"from":         from.String(),
		"to":           to.String(),
		"limit":        limit,
		"backwards":    backwardOrdering,
		"return_start": start.String(),
		"return_end":   end.String(),
	}).Info("Responding")

	// Respond with the events.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: messagesResp{
			Chunk: clientEvents,
			Start: start.String(),
			End:   end.String(),
		},
	}
}

// retrieveEvents retrieve events from the local database for a request on
// /messages. If there's not enough events to retrieve, it asks another
// homeserver in the room for older events.
// Returns an error if there was an issue talking to the database or with the
// remote homeserver.
func (r *messagesReq) retrieveEvents() (
	clientEvents []gomatrixserverlib.ClientEvent, start,
	end *types.PaginationToken, err error,
) {
	// Retrieve the events from the local database.
	streamEvents, err := r.db.GetEventsInRange(
		r.ctx, r.from, r.to, r.roomID, r.limit, r.backwardOrdering,
	)
	if err != nil {
		err = fmt.Errorf("GetEventsInRange: %w", err)
		return
	}

	var events []gomatrixserverlib.HeaderedEvent

	// There can be two reasons for streamEvents to be empty: either we've
	// reached the oldest event in the room (or the most recent one, depending
	// on the ordering), or we've reached a backward extremity.
	if len(streamEvents) == 0 {
		if events, err = r.handleEmptyEventsSlice(); err != nil {
			return
		}
	} else {
		if events, err = r.handleNonEmptyEventsSlice(streamEvents); err != nil {
			return
		}
	}

	// If we didn't get any event, we don't need to proceed any further.
	if len(events) == 0 {
		return []gomatrixserverlib.ClientEvent{}, r.from, r.to, nil
	}

	// Sort the events to ensure we send them in the right order. We currently
	// do that based on the event's timestamp.
	if r.backwardOrdering {
		sort.SliceStable(events, func(i int, j int) bool {
			// Backward ordering is antichronological (latest event to oldest
			// one).
			return sortEvents(&(events[j]), &(events[i]))
		})
	} else {
		sort.SliceStable(events, func(i int, j int) bool {
			// Forward ordering is chronological (oldest event to latest one).
			return sortEvents(&(events[i]), &(events[j]))
		})
	}

	// Convert all of the events into client events.
	clientEvents = gomatrixserverlib.HeaderedToClientEvents(events, gomatrixserverlib.FormatAll)
	// Get the position of the first and the last event in the room's topology.
	// This position is currently determined by the event's depth, so we could
	// also use it instead of retrieving from the database. However, if we ever
	// change the way topological positions are defined (as depth isn't the most
	// reliable way to define it), it would be easier and less troublesome to
	// only have to change it in one place, i.e. the database.
	startPos, err := r.db.EventPositionInTopology(
		r.ctx, events[0].EventID(),
	)
	if err != nil {
		err = fmt.Errorf("EventPositionInTopology: for start event %s: %w", events[0].EventID(), err)
		return
	}
	endPos, err := r.db.EventPositionInTopology(
		r.ctx, events[len(events)-1].EventID(),
	)
	if err != nil {
		err = fmt.Errorf("EventPositionInTopology: for end event %s: %w", events[len(events)-1].EventID(), err)
		return
	}
	// Generate pagination tokens to send to the client using the positions
	// retrieved previously.
	start = types.NewPaginationTokenFromTypeAndPosition(
		types.PaginationTokenTypeTopology, startPos, 0,
	)
	end = types.NewPaginationTokenFromTypeAndPosition(
		types.PaginationTokenTypeTopology, endPos, 0,
	)

	if r.backwardOrdering {
		// A stream/topological position is a cursor located between two events.
		// While they are identified in the code by the event on their right (if
		// we consider a left to right chronological order), tokens need to refer
		// to them by the event on their left, therefore we need to decrement the
		// end position we send in the response if we're going backward.
		end.PDUPosition--
	}

	// The lowest token value is 1, therefore we need to manually set it to that
	// value if we're below it.
	if end.PDUPosition < types.StreamPosition(1) {
		end.PDUPosition = types.StreamPosition(1)
	}

	return clientEvents, start, end, err
}

// handleEmptyEventsSlice handles the case where the initial request to the
// database returned an empty slice of events. It does so by checking whether
// the set is empty because we've reached a backward extremity, and if that is
// the case, by retrieving as much events as requested by backfilling from
// another homeserver.
// Returns an error if there was an issue talking with the database or
// backfilling.
func (r *messagesReq) handleEmptyEventsSlice() (
	events []gomatrixserverlib.HeaderedEvent, err error,
) {
	backwardExtremities, err := r.db.BackwardExtremitiesForRoom(r.ctx, r.roomID)

	// Check if we have backward extremities for this room.
	if len(backwardExtremities) > 0 {
		// If so, retrieve as much events as needed through backfilling.
		events, err = r.backfill(backwardExtremities, r.limit)
		if err != nil {
			return
		}
	} else {
		// If not, it means the slice was empty because we reached the room's
		// creation, so return an empty slice.
		events = []gomatrixserverlib.HeaderedEvent{}
	}

	return
}

// handleNonEmptyEventsSlice handles the case where the initial request to the
// database returned a non-empty slice of events. It does so by checking whether
// events are missing from the expected result, and retrieve missing events
// through backfilling if needed.
// Returns an error if there was an issue while backfilling.
func (r *messagesReq) handleNonEmptyEventsSlice(streamEvents []types.StreamEvent) (
	events []gomatrixserverlib.HeaderedEvent, err error,
) {
	// Check if we have enough events.
	isSetLargeEnough := len(streamEvents) >= r.limit
	if !isSetLargeEnough {
		// it might be fine we don't have up to 'limit' events, let's find out
		if r.backwardOrdering {
			if r.wasToProvided {
				// The condition in the SQL query is a strict "greater than" so
				// we need to check against to-1.
				streamPos := types.StreamPosition(streamEvents[len(streamEvents)-1].StreamPosition)
				isSetLargeEnough = (r.to.PDUPosition-1 == streamPos)
			}
		} else {
			streamPos := types.StreamPosition(streamEvents[0].StreamPosition)
			isSetLargeEnough = (r.from.PDUPosition-1 == streamPos)
		}
	}

	// Check if the slice contains a backward extremity.
	backwardExtremities, err := r.db.BackwardExtremitiesForRoom(r.ctx, r.roomID)
	if err != nil {
		return
	}

	// Backfill is needed if we've reached a backward extremity and need more
	// events. It's only needed if the direction is backward.
	if len(backwardExtremities) > 0 && !isSetLargeEnough && r.backwardOrdering {
		var pdus []gomatrixserverlib.HeaderedEvent
		// Only ask the remote server for enough events to reach the limit.
		pdus, err = r.backfill(backwardExtremities, r.limit-len(streamEvents))
		if err != nil {
			return
		}

		// Append the PDUs to the list to send back to the client.
		events = append(events, pdus...)
	}

	// Append the events ve previously retrieved locally.
	events = append(events, r.db.StreamEventsToEvents(nil, streamEvents)...)

	return
}

// backfill performs a backfill request over the federation on another
// homeserver in the room.
// See: https://matrix.org/docs/spec/server_server/latest#get-matrix-federation-v1-backfill-roomid
// It also stores the PDUs retrieved from the remote homeserver's response to
// the database.
// Returns with an empty string if the remote homeserver didn't return with any
// event, or if there is no remote homeserver to contact.
// Returns an error if there was an issue with retrieving the list of servers in
// the room or sending the request.
func (r *messagesReq) backfill(fromEventIDs []string, limit int) ([]gomatrixserverlib.HeaderedEvent, error) {
	srvToBackfillFrom, err := r.serverToBackfillFrom(fromEventIDs)
	if err != nil {
		return nil, fmt.Errorf("Cannot find server to backfill from: %w", err)
	}

	pdus := make([]gomatrixserverlib.HeaderedEvent, 0)

	// If the roomserver responded with at least one server that isn't us,
	// send it a request for backfill.
	util.GetLogger(r.ctx).WithField("server", srvToBackfillFrom).WithField("limit", limit).Info("Backfilling from server")
	txn, err := r.federation.Backfill(
		r.ctx, srvToBackfillFrom, r.roomID, limit, fromEventIDs,
	)
	if err != nil {
		return nil, err
	}

	for _, p := range txn.PDUs {
		pdus = append(pdus, p.Headered(gomatrixserverlib.RoomVersionV1))
	}
	util.GetLogger(r.ctx).WithField("server", srvToBackfillFrom).WithField("new_events", len(pdus)).Info("Storing new events from backfill")

	// Store the events in the database, while marking them as unfit to show
	// up in responses to sync requests.
	for _, pdu := range pdus {
		headered := pdu.Headered(gomatrixserverlib.RoomVersionV1)
		if _, err = r.db.WriteEvent(
			r.ctx,
			&headered,
			[]gomatrixserverlib.HeaderedEvent{},
			[]string{},
			[]string{},
			nil, true,
		); err != nil {
			return nil, err
		}
	}

	return pdus, nil
}

func (r *messagesReq) serverToBackfillFrom(fromEventIDs []string) (gomatrixserverlib.ServerName, error) {
	// Query the list of servers in the room when one of the backward extremities
	// was sent.
	var serversResponse api.QueryServersInRoomAtEventResponse
	serversRequest := api.QueryServersInRoomAtEventRequest{
		RoomID:  r.roomID,
		EventID: fromEventIDs[0],
	}
	if err := r.queryAPI.QueryServersInRoomAtEvent(r.ctx, &serversRequest, &serversResponse); err != nil {
		util.GetLogger(r.ctx).WithError(err).Warn("Failed to query servers in room at event, falling back to event sender")
		// FIXME: We shouldn't be doing this but in situations where we have already backfilled once
		// the query API doesn't work as backfilled events do not make it to the room server.
		// This means QueryServersInRoomAtEvent returns an error as it doesn't have the event ID in question.
		// We need to inject backfilled events into the room server and store them appropriately.
		events, err := r.db.Events(r.ctx, fromEventIDs)
		if err != nil {
			return "", err
		}
		if len(events) == 0 {
			// should be impossible as these event IDs are backwards extremities
			return "", fmt.Errorf("backfill: missing backwards extremities, event IDs: %s", fromEventIDs)
		}
		// The rationale here is that the last event was unlikely to be sent by us, so poke the server who sent it.
		// We shouldn't be doing this really, but as a heuristic it should work pretty well for now.
		for _, e := range events {
			_, srv, err := gomatrixserverlib.SplitID('@', e.Sender())
			if err != nil {
				util.GetLogger(r.ctx).WithError(err).Warn("Failed to extract domain from event sender")
				continue
			}
			if srv != r.cfg.Matrix.ServerName {
				return srv, nil
			}
		}
		// no valid events which have a remote server, fail.
		return "", err
	}

	// Use the first server from the response, except if that server is us.
	// In that case, use the second one if the roomserver responded with
	// enough servers. If not, use an empty string to prevent the backfill
	// from happening as there's no server to direct the request towards.
	// TODO: Be smarter at selecting the server to direct the request
	// towards.
	srvToBackfillFrom := serversResponse.Servers[0]
	if srvToBackfillFrom == r.cfg.Matrix.ServerName {
		if len(serversResponse.Servers) > 1 {
			srvToBackfillFrom = serversResponse.Servers[1]
		} else {
			util.GetLogger(r.ctx).Info("Not enough servers to backfill from")
			return "", nil
		}
	}
	return srvToBackfillFrom, nil
}

// setToDefault returns the default value for the "to" query parameter of a
// request to /messages if not provided. It defaults to either the earliest
// topological position (if we're going backward) or to the latest one (if we're
// going forward).
// Returns an error if there was an issue with retrieving the latest position
// from the database
func setToDefault(
	ctx context.Context, db storage.Database, backwardOrdering bool,
	roomID string,
) (to *types.PaginationToken, err error) {
	if backwardOrdering {
		// go 1 earlier than the first event so we correctly fetch the earliest event
		to = types.NewPaginationTokenFromTypeAndPosition(types.PaginationTokenTypeTopology, 0, 0)
	} else {
		var pos types.StreamPosition
		pos, err = db.MaxTopologicalPosition(ctx, roomID)
		if err != nil {
			return
		}

		to = types.NewPaginationTokenFromTypeAndPosition(types.PaginationTokenTypeTopology, pos, 0)
	}

	return
}

// sortEvents is a function to give to sort.SliceStable, and compares the
// timestamp of two Matrix events.
// Returns true if the first event happened before the second one, false
// otherwise.
func sortEvents(e1 *gomatrixserverlib.HeaderedEvent, e2 *gomatrixserverlib.HeaderedEvent) bool {
	t := e1.OriginServerTS().Time()
	return e2.OriginServerTS().Time().After(t)
}
