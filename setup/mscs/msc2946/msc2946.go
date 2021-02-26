// Copyright 2021 The Matrix.org Foundation C.I.C.
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

// Package msc2946 'Spaces Summary' implements https://github.com/matrix-org/matrix-doc/pull/2946
package msc2946

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	chttputil "github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	fs "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"
)

const (
	ConstCreateEventContentKey = "org.matrix.msc1772.type"
	ConstSpaceChildEventType   = "org.matrix.msc1772.space.child"
	ConstSpaceParentEventType  = "org.matrix.msc1772.space.parent"
)

// Defaults sets the request defaults
func Defaults(r *gomatrixserverlib.MSC2946SpacesRequest) {
	r.Limit = 2000
	r.MaxRoomsPerSpace = -1
}

// Enable this MSC
func Enable(
	base *setup.BaseDendrite, rsAPI roomserver.RoomserverInternalAPI, userAPI userapi.UserInternalAPI,
	fsAPI fs.FederationSenderInternalAPI, keyRing gomatrixserverlib.JSONVerifier,
) error {
	db, err := NewDatabase(&base.Cfg.MSCs.Database)
	if err != nil {
		return fmt.Errorf("Cannot enable MSC2946: %w", err)
	}
	hooks.Enable()
	hooks.Attach(hooks.KindNewEventPersisted, func(headeredEvent interface{}) {
		he := headeredEvent.(*gomatrixserverlib.HeaderedEvent)
		hookErr := db.StoreReference(context.Background(), he)
		if hookErr != nil {
			util.GetLogger(context.Background()).WithError(hookErr).WithField("event_id", he.EventID()).Error(
				"failed to StoreReference",
			)
		}
	})

	base.PublicClientAPIMux.Handle("/unstable/rooms/{roomID}/spaces",
		httputil.MakeAuthAPI("spaces", userAPI, spacesHandler(db, rsAPI, fsAPI, base.Cfg.Global.ServerName)),
	).Methods(http.MethodPost, http.MethodOptions)

	base.PublicFederationAPIMux.Handle("/unstable/spaces/{roomID}", httputil.MakeExternalAPI(
		"msc2946_fed_spaces", func(req *http.Request) util.JSONResponse {
			fedReq, errResp := gomatrixserverlib.VerifyHTTPRequest(
				req, time.Now(), base.Cfg.Global.ServerName, keyRing,
			)
			if fedReq == nil {
				return errResp
			}
			// Extract the room ID from the request. Sanity check request data.
			params, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			roomID := params["roomID"]
			return federatedSpacesHandler(req.Context(), fedReq, roomID, db, rsAPI, fsAPI, base.Cfg.Global.ServerName)
		},
	)).Methods(http.MethodPost, http.MethodOptions)
	return nil
}

func federatedSpacesHandler(
	ctx context.Context, fedReq *gomatrixserverlib.FederationRequest, roomID string, db Database,
	rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationSenderInternalAPI,
	thisServer gomatrixserverlib.ServerName,
) util.JSONResponse {
	inMemoryBatchCache := make(map[string]set)
	var r gomatrixserverlib.MSC2946SpacesRequest
	Defaults(&r)
	if err := json.Unmarshal(fedReq.Content(), &r); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}
	w := walker{
		req:        &r,
		rootRoomID: roomID,
		serverName: fedReq.Origin(),
		thisServer: thisServer,
		ctx:        ctx,

		db:                 db,
		rsAPI:              rsAPI,
		fsAPI:              fsAPI,
		inMemoryBatchCache: inMemoryBatchCache,
	}
	res := w.walk()
	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

func spacesHandler(
	db Database, rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationSenderInternalAPI,
	thisServer gomatrixserverlib.ServerName,
) func(*http.Request, *userapi.Device) util.JSONResponse {
	return func(req *http.Request, device *userapi.Device) util.JSONResponse {
		inMemoryBatchCache := make(map[string]set)
		// Extract the room ID from the request. Sanity check request data.
		params, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		roomID := params["roomID"]
		var r gomatrixserverlib.MSC2946SpacesRequest
		Defaults(&r)
		if resErr := chttputil.UnmarshalJSONRequest(req, &r); resErr != nil {
			return *resErr
		}
		w := walker{
			req:        &r,
			rootRoomID: roomID,
			caller:     device,
			thisServer: thisServer,
			ctx:        req.Context(),

			db:                 db,
			rsAPI:              rsAPI,
			fsAPI:              fsAPI,
			inMemoryBatchCache: inMemoryBatchCache,
		}
		res := w.walk()
		return util.JSONResponse{
			Code: 200,
			JSON: res,
		}
	}
}

type walker struct {
	req        *gomatrixserverlib.MSC2946SpacesRequest
	rootRoomID string
	caller     *userapi.Device
	serverName gomatrixserverlib.ServerName
	thisServer gomatrixserverlib.ServerName
	db         Database
	rsAPI      roomserver.RoomserverInternalAPI
	fsAPI      fs.FederationSenderInternalAPI
	ctx        context.Context

	// user ID|device ID|batch_num => event/room IDs sent to client
	inMemoryBatchCache map[string]set
	mu                 sync.Mutex
}

func (w *walker) roomIsExcluded(roomID string) bool {
	for _, exclRoom := range w.req.ExcludeRooms {
		if exclRoom == roomID {
			return true
		}
	}
	return false
}

func (w *walker) callerID() string {
	if w.caller != nil {
		return w.caller.UserID + "|" + w.caller.ID
	}
	return string(w.serverName)
}

func (w *walker) alreadySent(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	m, ok := w.inMemoryBatchCache[w.callerID()]
	if !ok {
		return false
	}
	return m[id]
}

func (w *walker) markSent(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	m := w.inMemoryBatchCache[w.callerID()]
	if m == nil {
		m = make(set)
	}
	m[id] = true
	w.inMemoryBatchCache[w.callerID()] = m
}

// nolint:gocyclo
func (w *walker) walk() *gomatrixserverlib.MSC2946SpacesResponse {
	var res gomatrixserverlib.MSC2946SpacesResponse
	// Begin walking the graph starting with the room ID in the request in a queue of unvisited rooms
	unvisited := []string{w.rootRoomID}
	processed := make(set)
	for len(unvisited) > 0 {
		roomID := unvisited[0]
		unvisited = unvisited[1:]
		// If this room has already been processed, skip. NB: do not remember this between calls
		if processed[roomID] || roomID == "" {
			continue
		}
		// Mark this room as processed.
		processed[roomID] = true

		// Collect rooms/events to send back (either locally or fetched via federation)
		var discoveredRooms []gomatrixserverlib.MSC2946Room
		var discoveredEvents []gomatrixserverlib.MSC2946StrippedEvent

		// If we know about this room and the caller is authorised (joined/world_readable) then pull
		// events locally
		if w.roomExists(roomID) && w.authorised(roomID) {
			// Get all `m.space.child` and `m.space.parent` state events for the room. *In addition*, get
			// all `m.space.child` and `m.space.parent` state events which *point to* (via `state_key` or `content.room_id`)
			// this room. This requires servers to store reverse lookups.
			events, err := w.references(roomID)
			if err != nil {
				util.GetLogger(w.ctx).WithError(err).WithField("room_id", roomID).Error("failed to extract references for room")
				continue
			}
			discoveredEvents = events

			pubRoom := w.publicRoomsChunk(roomID)
			roomType := ""
			create := w.stateEvent(roomID, gomatrixserverlib.MRoomCreate, "")
			if create != nil {
				// escape the `.`s so gjson doesn't think it's nested
				roomType = gjson.GetBytes(create.Content(), strings.ReplaceAll(ConstCreateEventContentKey, ".", `\.`)).Str
			}

			// Add the total number of events to `PublicRoomsChunk` under `num_refs`. Add `PublicRoomsChunk` to `rooms`.
			discoveredRooms = append(discoveredRooms, gomatrixserverlib.MSC2946Room{
				PublicRoom: *pubRoom,
				NumRefs:    len(discoveredEvents),
				RoomType:   roomType,
			})
		} else {
			// attempt to query this room over federation, as either we've never heard of it before
			// or we've left it and hence are not authorised (but info may be exposed regardless)
			fedRes, err := w.federatedRoomInfo(roomID)
			if err != nil {
				util.GetLogger(w.ctx).WithError(err).WithField("room_id", roomID).Errorf("failed to query federated spaces")
				continue
			}
			if fedRes != nil {
				discoveredRooms = fedRes.Rooms
				discoveredEvents = fedRes.Events
			}
		}

		// If this room has not ever been in `rooms` (across multiple requests), send it now
		for _, room := range discoveredRooms {
			if !w.alreadySent(room.RoomID) && !w.roomIsExcluded(room.RoomID) {
				res.Rooms = append(res.Rooms, room)
				w.markSent(room.RoomID)
			}
		}

		uniqueRooms := make(set)

		// If this is the root room from the original request, insert all these events into `events` if
		// they haven't been added before (across multiple requests).
		if w.rootRoomID == roomID {
			for _, ev := range discoveredEvents {
				if !w.alreadySent(eventKey(&ev)) {
					res.Events = append(res.Events, ev)
					uniqueRooms[ev.RoomID] = true
					uniqueRooms[spaceTargetStripped(&ev)] = true
					w.markSent(eventKey(&ev))
				}
			}
		} else {
			// Else add them to `events` honouring the `limit` and `max_rooms_per_space` values. If either
			// are exceeded, stop adding events. If the event has already been added, do not add it again.
			numAdded := 0
			for _, ev := range discoveredEvents {
				if w.req.Limit > 0 && len(res.Events) >= w.req.Limit {
					break
				}
				if w.req.MaxRoomsPerSpace > 0 && numAdded >= w.req.MaxRoomsPerSpace {
					break
				}
				if w.alreadySent(eventKey(&ev)) {
					continue
				}
				// Skip the room if it's part of exclude_rooms but ONLY IF the source matches, as we still
				// want to catch arrows which point to excluded rooms.
				if w.roomIsExcluded(ev.RoomID) {
					continue
				}
				res.Events = append(res.Events, ev)
				uniqueRooms[ev.RoomID] = true
				uniqueRooms[spaceTargetStripped(&ev)] = true
				w.markSent(eventKey(&ev))
				// we don't distinguish between child state events and parent state events for the purposes of
				// max_rooms_per_space, maybe we should?
				numAdded++
			}
		}

		// For each referenced room ID in the events being returned to the caller (both parent and child)
		// add the room ID to the queue of unvisited rooms. Loop from the beginning.
		for roomID := range uniqueRooms {
			unvisited = append(unvisited, roomID)
		}
	}
	return &res
}

func (w *walker) stateEvent(roomID, evType, stateKey string) *gomatrixserverlib.HeaderedEvent {
	var queryRes roomserver.QueryCurrentStateResponse
	tuple := gomatrixserverlib.StateKeyTuple{
		EventType: evType,
		StateKey:  stateKey,
	}
	err := w.rsAPI.QueryCurrentState(w.ctx, &roomserver.QueryCurrentStateRequest{
		RoomID:      roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{tuple},
	}, &queryRes)
	if err != nil {
		return nil
	}
	return queryRes.StateEvents[tuple]
}

func (w *walker) publicRoomsChunk(roomID string) *gomatrixserverlib.PublicRoom {
	pubRooms, err := roomserver.PopulatePublicRooms(w.ctx, []string{roomID}, w.rsAPI)
	if err != nil {
		util.GetLogger(w.ctx).WithError(err).Error("failed to PopulatePublicRooms")
		return nil
	}
	if len(pubRooms) == 0 {
		return nil
	}
	return &pubRooms[0]
}

// federatedRoomInfo returns more of the spaces graph from another server. Returns nil if this was
// unsuccessful.
func (w *walker) federatedRoomInfo(roomID string) (*gomatrixserverlib.MSC2946SpacesResponse, error) {
	// only do federated requests for client requests
	if w.caller == nil {
		return nil, nil
	}
	// extract events which point to this room ID and extract their vias
	events, err := w.db.References(w.ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get References events: %w", err)
	}
	vias := make(set)
	for _, ev := range events {
		if ev.StateKeyEquals(roomID) {
			// event points at this room, extract vias
			content := struct {
				Vias []string `json:"via"`
			}{}
			if err = json.Unmarshal(ev.Content(), &content); err != nil {
				continue // silently ignore corrupted state events
			}
			for _, v := range content.Vias {
				vias[v] = true
			}
		}
	}
	util.GetLogger(w.ctx).Infof("Querying federatedRoomInfo via %+v", vias)
	ctx := context.Background()
	// query more of the spaces graph using these servers
	for serverName := range vias {
		if serverName == string(w.thisServer) {
			continue
		}
		res, err := w.fsAPI.MSC2946Spaces(ctx, gomatrixserverlib.ServerName(serverName), roomID, gomatrixserverlib.MSC2946SpacesRequest{
			Limit:            w.req.Limit,
			MaxRoomsPerSpace: w.req.MaxRoomsPerSpace,
		})
		if err != nil {
			util.GetLogger(w.ctx).WithError(err).Warnf("failed to call MSC2946Spaces on server %s", serverName)
			continue
		}
		return &res, nil
	}
	return nil, nil
}

func (w *walker) roomExists(roomID string) bool {
	var queryRes roomserver.QueryServerJoinedToRoomResponse
	err := w.rsAPI.QueryServerJoinedToRoom(w.ctx, &roomserver.QueryServerJoinedToRoomRequest{
		RoomID:     roomID,
		ServerName: w.thisServer,
	}, &queryRes)
	if err != nil {
		util.GetLogger(w.ctx).WithError(err).Error("failed to QueryServerJoinedToRoom")
		return false
	}
	// if the room exists but we aren't in the room then we might have stale data so we want to fetch
	// it fresh via federation
	return queryRes.RoomExists && queryRes.IsInRoom
}

// authorised returns true iff the user is joined this room or the room is world_readable
func (w *walker) authorised(roomID string) bool {
	if w.caller != nil {
		return w.authorisedUser(roomID)
	}
	return w.authorisedServer(roomID)
}

// authorisedServer returns true iff the server is joined this room or the room is world_readable
func (w *walker) authorisedServer(roomID string) bool {
	// Check history visibility first
	hisVisTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomHistoryVisibility,
		StateKey:  "",
	}
	var queryRoomRes roomserver.QueryCurrentStateResponse
	err := w.rsAPI.QueryCurrentState(w.ctx, &roomserver.QueryCurrentStateRequest{
		RoomID: roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			hisVisTuple,
		},
	}, &queryRoomRes)
	if err != nil {
		util.GetLogger(w.ctx).WithError(err).Error("failed to QueryCurrentState")
		return false
	}
	hisVisEv := queryRoomRes.StateEvents[hisVisTuple]
	if hisVisEv != nil {
		hisVis, _ := hisVisEv.HistoryVisibility()
		if hisVis == "world_readable" {
			return true
		}
	}
	// check if server is joined to the room
	var queryRes fs.QueryJoinedHostServerNamesInRoomResponse
	err = w.fsAPI.QueryJoinedHostServerNamesInRoom(w.ctx, &fs.QueryJoinedHostServerNamesInRoomRequest{
		RoomID: roomID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(w.ctx).WithError(err).Error("failed to QueryJoinedHostServerNamesInRoom")
		return false
	}
	for _, srv := range queryRes.ServerNames {
		if srv == w.serverName {
			return true
		}
	}
	return false
}

// authorisedUser returns true iff the user is joined this room or the room is world_readable
func (w *walker) authorisedUser(roomID string) bool {
	hisVisTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomHistoryVisibility,
		StateKey:  "",
	}
	roomMemberTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomMember,
		StateKey:  w.caller.UserID,
	}
	var queryRes roomserver.QueryCurrentStateResponse
	err := w.rsAPI.QueryCurrentState(w.ctx, &roomserver.QueryCurrentStateRequest{
		RoomID: roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			hisVisTuple, roomMemberTuple,
		},
	}, &queryRes)
	if err != nil {
		util.GetLogger(w.ctx).WithError(err).Error("failed to QueryCurrentState")
		return false
	}
	memberEv := queryRes.StateEvents[roomMemberTuple]
	hisVisEv := queryRes.StateEvents[hisVisTuple]
	if memberEv != nil {
		membership, _ := memberEv.Membership()
		if membership == gomatrixserverlib.Join {
			return true
		}
	}
	if hisVisEv != nil {
		hisVis, _ := hisVisEv.HistoryVisibility()
		if hisVis == "world_readable" {
			return true
		}
	}
	return false
}

// references returns all references pointing to or from this room.
func (w *walker) references(roomID string) ([]gomatrixserverlib.MSC2946StrippedEvent, error) {
	events, err := w.db.References(w.ctx, roomID)
	if err != nil {
		return nil, err
	}
	el := make([]gomatrixserverlib.MSC2946StrippedEvent, 0, len(events))
	for _, ev := range events {
		// only return events that have a `via` key as per MSC1772
		// else we'll incorrectly walk redacted events (as the link
		// is in the state_key)
		if gjson.GetBytes(ev.Content(), "via").Exists() {
			strip := stripped(ev.Event)
			if strip == nil {
				continue
			}
			el = append(el, *strip)
		}
	}
	return el, nil
}

type set map[string]bool

func stripped(ev *gomatrixserverlib.Event) *gomatrixserverlib.MSC2946StrippedEvent {
	if ev.StateKey() == nil {
		return nil
	}
	return &gomatrixserverlib.MSC2946StrippedEvent{
		Type:     ev.Type(),
		StateKey: *ev.StateKey(),
		Content:  ev.Content(),
		Sender:   ev.Sender(),
		RoomID:   ev.RoomID(),
	}
}

func eventKey(event *gomatrixserverlib.MSC2946StrippedEvent) string {
	return event.RoomID + "|" + event.Type + "|" + event.StateKey
}

func spaceTargetStripped(event *gomatrixserverlib.MSC2946StrippedEvent) string {
	if event.StateKey == "" {
		return "" // no-op
	}
	switch event.Type {
	case ConstSpaceParentEventType:
		return event.StateKey
	case ConstSpaceChildEventType:
		return event.StateKey
	}
	return ""
}
