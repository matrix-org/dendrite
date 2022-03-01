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

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type messagesReq struct {
	ctx              context.Context
	db               storage.Database
	rsAPI            api.RoomserverInternalAPI
	federation       *gomatrixserverlib.FederationClient
	cfg              *config.SyncAPI
	roomID           string
	from             *types.TopologyToken
	to               *types.TopologyToken
	fromStream       *types.StreamingToken
	device           *userapi.Device
	wasToProvided    bool
	backwardOrdering bool
	filter           *gomatrixserverlib.RoomEventFilter
}

type messagesResp struct {
	Start       string                          `json:"start"`
	StartStream string                          `json:"start_stream,omitempty"` // NOTSPEC: so clients can hit /messages then immediately /sync with a latest sync token
	End         string                          `json:"end"`
	Chunk       []gomatrixserverlib.ClientEvent `json:"chunk"`
	State       []gomatrixserverlib.ClientEvent `json:"state"`
}

// OnIncomingMessagesRequest implements the /messages endpoint from the
// client-server API.
// See: https://matrix.org/docs/spec/client_server/latest.html#get-matrix-client-r0-rooms-roomid-messages
func OnIncomingMessagesRequest(
	req *http.Request, db storage.Database, roomID string, device *userapi.Device,
	federation *gomatrixserverlib.FederationClient,
	rsAPI api.RoomserverInternalAPI,
	cfg *config.SyncAPI,
	srp *sync.RequestPool,
) util.JSONResponse {
	var err error

	// check if the user has already forgotten about this room
	isForgotten, err := checkIsRoomForgotten(req.Context(), roomID, device.UserID, rsAPI)
	if err != nil {
		return jsonerror.InternalServerError()
	}

	if isForgotten {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("user already forgot about this room"),
		}
	}

	filter, err := parseRoomEventFilter(req)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("unable to parse filter"),
		}
	}

	// Extract parameters from the request's URL.
	// Pagination tokens.
	var fromStream *types.StreamingToken
	fromQuery := req.URL.Query().Get("from")
	emptyFromSupplied := fromQuery == ""
	if emptyFromSupplied {
		// NOTSPEC: We will pretend they used the latest sync token if no ?from= was provided.
		// We do this to allow clients to get messages without having to call `/sync` e.g Cerulean
		currPos := srp.Notifier.CurrentPosition()
		fromQuery = currPos.String()
	}

	from, err := types.NewTopologyTokenFromString(fromQuery)
	if err != nil {
		fs, err2 := types.NewStreamTokenFromString(fromQuery)
		fromStream = &fs
		if err2 != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidArgumentValue("Invalid from parameter: " + err2.Error()),
			}
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
	var to types.TopologyToken
	wasToProvided := true
	if s := req.URL.Query().Get("to"); len(s) > 0 {
		to, err = types.NewTopologyTokenFromString(s)
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
		rsAPI:            rsAPI,
		federation:       federation,
		cfg:              cfg,
		roomID:           roomID,
		from:             &from,
		to:               &to,
		fromStream:       fromStream,
		wasToProvided:    wasToProvided,
		filter:           filter,
		backwardOrdering: backwardOrdering,
		device:           device,
	}

	clientEvents, start, end, err := mReq.retrieveEvents()
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("mreq.retrieveEvents failed")
		return jsonerror.InternalServerError()
	}

	// at least fetch the membership events for the users returned in chunk if LazyLoadMembers is set
	state := []gomatrixserverlib.ClientEvent{}
	if filter.LazyLoadMembers {
		memberShipToUser := make(map[string]*gomatrixserverlib.HeaderedEvent)
		for _, evt := range clientEvents {
			memberShip, err := db.GetStateEvent(req.Context(), roomID, gomatrixserverlib.MRoomMember, evt.Sender)
			if err != nil {
				util.GetLogger(req.Context()).WithError(err).Error("failed to get membership event for user")
				continue
			}
			memberShipToUser[evt.Sender] = memberShip
		}
		for _, evt := range memberShipToUser {
			state = append(state, gomatrixserverlib.HeaderedToClientEvent(evt, gomatrixserverlib.FormatAll))
		}
	}

	util.GetLogger(req.Context()).WithFields(logrus.Fields{
		"from":         from.String(),
		"to":           to.String(),
		"limit":        filter.Limit,
		"backwards":    backwardOrdering,
		"return_start": start.String(),
		"return_end":   end.String(),
	}).Info("Responding")

	res := messagesResp{
		Chunk: clientEvents,
		Start: start.String(),
		End:   end.String(),
		State: state,
	}
	if emptyFromSupplied {
		res.StartStream = fromStream.String()
	}

	// Respond with the events.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}

func checkIsRoomForgotten(ctx context.Context, roomID, userID string, rsAPI api.RoomserverInternalAPI) (bool, error) {
	req := api.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: userID,
	}
	resp := api.QueryMembershipForUserResponse{}
	if err := rsAPI.QueryMembershipForUser(ctx, &req, &resp); err != nil {
		return false, err
	}

	return resp.IsRoomForgotten, nil
}

// retrieveEvents retrieves events from the local database for a request on
// /messages. If there's not enough events to retrieve, it asks another
// homeserver in the room for older events.
// Returns an error if there was an issue talking to the database or with the
// remote homeserver.
func (r *messagesReq) retrieveEvents() (
	clientEvents []gomatrixserverlib.ClientEvent, start,
	end types.TopologyToken, err error,
) {
	eventFilter := r.filter

	// Retrieve the events from the local database.
	var streamEvents []types.StreamEvent
	if r.fromStream != nil {
		toStream := r.to.StreamToken()
		streamEvents, err = r.db.GetEventsInStreamingRange(
			r.ctx, r.fromStream, &toStream, r.roomID, eventFilter, r.backwardOrdering,
		)
	} else {
		streamEvents, err = r.db.GetEventsInTopologicalRange(
			r.ctx, r.from, r.to, r.roomID, eventFilter.Limit, r.backwardOrdering,
		)
	}
	if err != nil {
		err = fmt.Errorf("GetEventsInRange: %w", err)
		return
	}

	var events []*gomatrixserverlib.HeaderedEvent
	util.GetLogger(r.ctx).WithField("start", start).WithField("end", end).Infof("Fetched %d events locally", len(streamEvents))

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
		return []gomatrixserverlib.ClientEvent{}, *r.from, *r.to, nil
	}

	// Get the position of the first and the last event in the room's topology.
	// This position is currently determined by the event's depth, so we could
	// also use it instead of retrieving from the database. However, if we ever
	// change the way topological positions are defined (as depth isn't the most
	// reliable way to define it), it would be easier and less troublesome to
	// only have to change it in one place, i.e. the database.
	start, end, err = r.getStartEnd(events)

	// Sort the events to ensure we send them in the right order.
	if r.backwardOrdering {
		// This reverses the array from old->new to new->old
		reversed := func(in []*gomatrixserverlib.HeaderedEvent) []*gomatrixserverlib.HeaderedEvent {
			out := make([]*gomatrixserverlib.HeaderedEvent, len(in))
			for i := 0; i < len(in); i++ {
				out[i] = in[len(in)-i-1]
			}
			return out
		}
		events = reversed(events)
	}
	events = r.filterHistoryVisible(events)
	if len(events) == 0 {
		return []gomatrixserverlib.ClientEvent{}, *r.from, *r.to, nil
	}

	// Convert all of the events into client events.
	clientEvents = gomatrixserverlib.HeaderedToClientEvents(events, gomatrixserverlib.FormatAll)
	return clientEvents, start, end, err
}

func (r *messagesReq) filterHistoryVisible(events []*gomatrixserverlib.HeaderedEvent) []*gomatrixserverlib.HeaderedEvent {
	// TODO FIXME: We don't fully implement history visibility yet. To avoid leaking events which the
	// user shouldn't see, we check the recent events and remove any prior to the join event of the user
	// which is equiv to history_visibility: joined
	joinEventIndex := -1
	for i, ev := range events {
		if ev.Type() == gomatrixserverlib.MRoomMember && ev.StateKeyEquals(r.device.UserID) {
			membership, _ := ev.Membership()
			if membership == "join" {
				joinEventIndex = i
				break
			}
		}
	}

	var result []*gomatrixserverlib.HeaderedEvent
	var eventsToCheck []*gomatrixserverlib.HeaderedEvent
	if joinEventIndex != -1 {
		if r.backwardOrdering {
			result = events[:joinEventIndex+1]
			eventsToCheck = append(eventsToCheck, result[0])
		} else {
			result = events[joinEventIndex:]
			eventsToCheck = append(eventsToCheck, result[len(result)-1])
		}
	} else {
		eventsToCheck = []*gomatrixserverlib.HeaderedEvent{events[0], events[len(events)-1]}
		result = events
	}
	// make sure the user was in the room for both the earliest and latest events, we need this because
	// some backpagination results will not have the join event (e.g if they hit /messages at the join event itself)
	wasJoined := true
	for _, ev := range eventsToCheck {
		var queryRes api.QueryStateAfterEventsResponse
		err := r.rsAPI.QueryStateAfterEvents(r.ctx, &api.QueryStateAfterEventsRequest{
			RoomID:       ev.RoomID(),
			PrevEventIDs: ev.PrevEventIDs(),
			StateToFetch: []gomatrixserverlib.StateKeyTuple{
				{EventType: gomatrixserverlib.MRoomMember, StateKey: r.device.UserID},
				{EventType: gomatrixserverlib.MRoomHistoryVisibility, StateKey: ""},
			},
		}, &queryRes)
		if err != nil {
			wasJoined = false
			break
		}
		var hisVisEvent, membershipEvent *gomatrixserverlib.HeaderedEvent
		for i := range queryRes.StateEvents {
			switch queryRes.StateEvents[i].Type() {
			case gomatrixserverlib.MRoomMember:
				membershipEvent = queryRes.StateEvents[i]
			case gomatrixserverlib.MRoomHistoryVisibility:
				hisVisEvent = queryRes.StateEvents[i]
			}
		}
		if hisVisEvent == nil {
			return events // apply no filtering as it defaults to Shared.
		}
		hisVis, _ := hisVisEvent.HistoryVisibility()
		if hisVis == "shared" || hisVis == "world_readable" {
			return events // apply no filtering
		}
		if membershipEvent == nil {
			wasJoined = false
			break
		}
		membership, err := membershipEvent.Membership()
		if err != nil {
			wasJoined = false
			break
		}
		if membership != "join" {
			wasJoined = false
			break
		}
	}
	if !wasJoined {
		util.GetLogger(r.ctx).WithField("num_events", len(events)).Warnf("%s was not joined to room during these events, omitting them", r.device.UserID)
		return []*gomatrixserverlib.HeaderedEvent{}
	}
	return result
}

func (r *messagesReq) getStartEnd(events []*gomatrixserverlib.HeaderedEvent) (start, end types.TopologyToken, err error) {
	if r.backwardOrdering {
		start = *r.from
		if events[len(events)-1].Type() == gomatrixserverlib.MRoomCreate {
			// NOTSPEC: We've hit the beginning of the room so there's really nowhere
			// else to go. This seems to fix Element iOS from looping on /messages endlessly.
			end = types.TopologyToken{}
		} else {
			end, err = r.db.EventPositionInTopology(
				r.ctx, events[0].EventID(),
			)
			// A stream/topological position is a cursor located between two events.
			// While they are identified in the code by the event on their right (if
			// we consider a left to right chronological order), tokens need to refer
			// to them by the event on their left, therefore we need to decrement the
			// end position we send in the response if we're going backward.
			end.Decrement()
		}
	} else {
		start = *r.from
		end, err = r.db.EventPositionInTopology(
			r.ctx, events[len(events)-1].EventID(),
		)
	}
	if err != nil {
		err = fmt.Errorf("EventPositionInTopology: for end event %s: %w", events[len(events)-1].EventID(), err)
		return
	}
	return
}

// handleEmptyEventsSlice handles the case where the initial request to the
// database returned an empty slice of events. It does so by checking whether
// the set is empty because we've reached a backward extremity, and if that is
// the case, by retrieving as much events as requested by backfilling from
// another homeserver.
// Returns an error if there was an issue talking with the database or
// backfilling.
func (r *messagesReq) handleEmptyEventsSlice() (
	events []*gomatrixserverlib.HeaderedEvent, err error,
) {
	backwardExtremities, err := r.db.BackwardExtremitiesForRoom(r.ctx, r.roomID)

	// Check if we have backward extremities for this room.
	if len(backwardExtremities) > 0 {
		// If so, retrieve as much events as needed through backfilling.
		events, err = r.backfill(r.roomID, backwardExtremities, r.filter.Limit)
		if err != nil {
			return
		}
	} else {
		// If not, it means the slice was empty because we reached the room's
		// creation, so return an empty slice.
		events = []*gomatrixserverlib.HeaderedEvent{}
	}

	return
}

// handleNonEmptyEventsSlice handles the case where the initial request to the
// database returned a non-empty slice of events. It does so by checking whether
// events are missing from the expected result, and retrieve missing events
// through backfilling if needed.
// Returns an error if there was an issue while backfilling.
func (r *messagesReq) handleNonEmptyEventsSlice(streamEvents []types.StreamEvent) (
	events []*gomatrixserverlib.HeaderedEvent, err error,
) {
	// Check if we have enough events.
	isSetLargeEnough := len(streamEvents) >= r.filter.Limit
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
		var pdus []*gomatrixserverlib.HeaderedEvent
		// Only ask the remote server for enough events to reach the limit.
		pdus, err = r.backfill(r.roomID, backwardExtremities, r.filter.Limit-len(streamEvents))
		if err != nil {
			return
		}

		// Append the PDUs to the list to send back to the client.
		events = append(events, pdus...)
	}

	// Append the events ve previously retrieved locally.
	events = append(events, r.db.StreamEventsToEvents(nil, streamEvents)...)
	sort.Sort(eventsByDepth(events))

	return
}

type eventsByDepth []*gomatrixserverlib.HeaderedEvent

func (e eventsByDepth) Len() int {
	return len(e)
}
func (e eventsByDepth) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}
func (e eventsByDepth) Less(i, j int) bool {
	return e[i].Depth() < e[j].Depth()
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
func (r *messagesReq) backfill(roomID string, backwardsExtremities map[string][]string, limit int) ([]*gomatrixserverlib.HeaderedEvent, error) {
	var res api.PerformBackfillResponse
	err := r.rsAPI.PerformBackfill(context.Background(), &api.PerformBackfillRequest{
		RoomID:               roomID,
		BackwardsExtremities: backwardsExtremities,
		Limit:                limit,
		ServerName:           r.cfg.Matrix.ServerName,
	}, &res)
	if err != nil {
		return nil, fmt.Errorf("PerformBackfill failed: %w", err)
	}
	util.GetLogger(r.ctx).WithField("new_events", len(res.Events)).Info("Storing new events from backfill")

	// TODO: we should only be inserting events into the database from the roomserver's kafka output stream.
	// Currently, this can race with live events for the room and cause problems. It's also just a bit unclear
	// when you have multiple entry points to write events.

	// we have to order these by depth, starting with the lowest because otherwise the topology tokens
	// will skip over events that have the same depth but different stream positions due to the query which is:
	//  - anything less than the depth OR
	//  - anything with the same depth and a lower stream position.
	sort.Sort(eventsByDepth(res.Events))

	// Store the events in the database, while marking them as unfit to show
	// up in responses to sync requests.
	for i := range res.Events {
		_, err = r.db.WriteEvent(
			context.Background(),
			res.Events[i],
			[]*gomatrixserverlib.HeaderedEvent{},
			[]string{},
			[]string{},
			nil, true,
		)
		if err != nil {
			return nil, err
		}
	}

	// we may have got more than the requested limit so resize now
	events := res.Events
	if len(events) > limit {
		// last `limit` events
		events = events[len(events)-limit:]
	}

	return events, nil
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
) (to types.TopologyToken, err error) {
	if backwardOrdering {
		// go 1 earlier than the first event so we correctly fetch the earliest event
		// this is because Database.GetEventsInTopologicalRange is exclusive of the lower-bound.
		to = types.TopologyToken{}
	} else {
		to, err = db.MaxTopologicalPosition(ctx, roomID)
	}

	return
}
