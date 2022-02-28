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
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	fs "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"
)

const (
	ConstCreateEventContentKey        = "type"
	ConstCreateEventContentValueSpace = "m.space"
	ConstSpaceChildEventType          = "m.space.child"
	ConstSpaceParentEventType         = "m.space.parent"
)

type MSC2946ClientResponse struct {
	Rooms     []gomatrixserverlib.MSC2946Room `json:"rooms"`
	NextBatch string                          `json:"next_batch"`
}

// Enable this MSC
func Enable(
	base *base.BaseDendrite, rsAPI roomserver.RoomserverInternalAPI, userAPI userapi.UserInternalAPI,
	fsAPI fs.FederationInternalAPI, keyRing gomatrixserverlib.JSONVerifier,
) error {
	db, err := NewDatabase(&base.Cfg.MSCs.Database)
	if err != nil {
		return fmt.Errorf("cannot enable MSC2946: %w", err)
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
	clientAPI := httputil.MakeAuthAPI("spaces", userAPI, spacesHandler(db, rsAPI, fsAPI, base.Cfg.Global.ServerName))
	base.PublicClientAPIMux.Handle("/v1/rooms/{roomID}/hierarchy", clientAPI).Methods(http.MethodGet, http.MethodOptions)
	base.PublicClientAPIMux.Handle("/unstable/org.matrix.msc2946/rooms/{roomID}/hierarchy", clientAPI).Methods(http.MethodGet, http.MethodOptions)

	fedAPI := httputil.MakeExternalAPI(
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
	)
	base.PublicFederationAPIMux.Handle("/unstable/org.matrix.msc2946/hierarchy/{roomID}", fedAPI).Methods(http.MethodGet)
	base.PublicFederationAPIMux.Handle("/v1/hierarchy/{roomID}", fedAPI).Methods(http.MethodGet)
	return nil
}

func federatedSpacesHandler(
	ctx context.Context, fedReq *gomatrixserverlib.FederationRequest, roomID string, db Database,
	rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationInternalAPI,
	thisServer gomatrixserverlib.ServerName,
) util.JSONResponse {
	inMemoryBatchCache := make(map[string]set)
	u, err := url.Parse(fedReq.RequestURI())
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidParam("bad request uri"),
		}
	}

	w := walker{
		rootRoomID:    roomID,
		serverName:    fedReq.Origin(),
		thisServer:    thisServer,
		ctx:           ctx,
		suggestedOnly: u.Query().Get("suggested_only") == "true",
		limit:         1000,
		// The main difference is that it does not recurse into spaces and does not support pagination.
		// This is somewhat equivalent to a Client-Server request with a max_depth=1.
		maxDepth: 1,

		db:                 db,
		rsAPI:              rsAPI,
		fsAPI:              fsAPI,
		inMemoryBatchCache: inMemoryBatchCache,
	}
	return w.walk()
}

func spacesHandler(
	db Database, rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationInternalAPI,
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
		w := walker{
			suggestedOnly: req.URL.Query().Get("suggested_only") == "true",
			limit:         parseInt(req.URL.Query().Get("limit"), 1000),
			maxDepth:      parseInt(req.URL.Query().Get("max_depth"), -1),
			rootRoomID:    roomID,
			caller:        device,
			thisServer:    thisServer,
			ctx:           req.Context(),

			db:                 db,
			rsAPI:              rsAPI,
			fsAPI:              fsAPI,
			inMemoryBatchCache: inMemoryBatchCache,
		}
		return w.walk()
	}
}

type walker struct {
	rootRoomID    string
	caller        *userapi.Device
	serverName    gomatrixserverlib.ServerName
	thisServer    gomatrixserverlib.ServerName
	db            Database
	rsAPI         roomserver.RoomserverInternalAPI
	fsAPI         fs.FederationInternalAPI
	ctx           context.Context
	suggestedOnly bool
	limit         int
	maxDepth      int

	// user ID|device ID|batch_num => event/room IDs sent to client
	inMemoryBatchCache map[string]set
	mu                 sync.Mutex
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

type roomVisit struct {
	roomID string
	depth  int
}

func (w *walker) walk() util.JSONResponse {
	if !w.authorised(w.rootRoomID) {
		if w.caller != nil {
			// CS API format
			return util.JSONResponse{
				Code: 403,
				JSON: jsonerror.Forbidden("room is unknown/forbidden"),
			}
		} else {
			// SS API format
			return util.JSONResponse{
				Code: 404,
				JSON: jsonerror.NotFound("room is unknown/forbidden"),
			}
		}
	}

	var discoveredRooms []gomatrixserverlib.MSC2946Room

	// Begin walking the graph starting with the room ID in the request in a queue of unvisited rooms
	// Depth first -> stack data structure
	unvisited := []roomVisit{{
		roomID: w.rootRoomID,
		depth:  0,
	}}
	processed := make(set)
	for len(unvisited) > 0 {
		if len(discoveredRooms) > w.limit {
			break
		}

		// pop the stack
		rv := unvisited[len(unvisited)-1]
		unvisited = unvisited[:len(unvisited)-1]
		// If this room has already been processed, skip.
		// If this room exceeds the specified depth, skip.
		if processed[rv.roomID] || rv.roomID == "" || (w.maxDepth > 0 && rv.depth > w.maxDepth) {
			continue
		}
		// Mark this room as processed.
		processed[rv.roomID] = true

		// if this room is not a space room, skip.
		var roomType string
		create := w.stateEvent(rv.roomID, gomatrixserverlib.MRoomCreate, "")
		if create != nil {
			// escape the `.`s so gjson doesn't think it's nested
			roomType = gjson.GetBytes(create.Content(), strings.ReplaceAll(ConstCreateEventContentKey, ".", `\.`)).Str
		}

		// Collect rooms/events to send back (either locally or fetched via federation)
		var discoveredChildEvents []gomatrixserverlib.MSC2946StrippedEvent

		// If we know about this room and the caller is authorised (joined/world_readable) then pull
		// events locally
		if w.roomExists(rv.roomID) && w.authorised(rv.roomID) {
			// Get all `m.space.child` state events for this room
			events, err := w.childReferences(rv.roomID)
			if err != nil {
				util.GetLogger(w.ctx).WithError(err).WithField("room_id", rv.roomID).Error("failed to extract references for room")
				continue
			}
			discoveredChildEvents = events

			pubRoom := w.publicRoomsChunk(rv.roomID)

			discoveredRooms = append(discoveredRooms, gomatrixserverlib.MSC2946Room{
				PublicRoom:    *pubRoom,
				RoomType:      roomType,
				ChildrenState: events,
			})
		} else {
			// attempt to query this room over federation, as either we've never heard of it before
			// or we've left it and hence are not authorised (but info may be exposed regardless)
			fedRes, err := w.federatedRoomInfo(rv.roomID)
			if err != nil {
				util.GetLogger(w.ctx).WithError(err).WithField("room_id", rv.roomID).Errorf("failed to query federated spaces")
				continue
			}
			if fedRes != nil {
				discoveredChildEvents = fedRes.Room.ChildrenState
				discoveredRooms = append(discoveredRooms, fedRes.Room)
			}
		}

		// mark processed rooms for pagination purposes
		for _, room := range discoveredRooms {
			if !w.alreadySent(room.RoomID) {
				w.markSent(room.RoomID)
			}
		}

		// don't walk the children
		// if the parent is not a space room
		if roomType != ConstCreateEventContentValueSpace {
			continue
		}

		uniqueRooms := make(set)
		for _, ev := range discoveredChildEvents {
			uniqueRooms[ev.StateKey] = true
		}

		// For each referenced room ID in the child events being returned to the caller
		// add the room ID to the queue of unvisited rooms. Loop from the beginning.
		for roomID := range uniqueRooms {
			unvisited = append(unvisited, roomVisit{
				roomID: roomID,
				depth:  rv.depth + 1,
			})
		}
	}
	if w.caller != nil {
		// return CS API format
		return util.JSONResponse{
			Code: 200,
			JSON: MSC2946ClientResponse{
				Rooms: discoveredRooms,
			},
		}
	}
	// return SS API format
	// the first discovered room will be the room asked for, and subsequent ones the depth=1 children
	if len(discoveredRooms) == 0 {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("room is unknown/forbidden"),
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: gomatrixserverlib.MSC2946SpacesResponse{
			Room:     discoveredRooms[0],
			Children: discoveredRooms[1:],
		},
	}
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
	events, err := w.db.ChildReferences(w.ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ChildReferences events: %w", err)
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
		res, err := w.fsAPI.MSC2946Spaces(ctx, gomatrixserverlib.ServerName(serverName), roomID, w.suggestedOnly)
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

// references returns all child references pointing to or from this room.
func (w *walker) childReferences(roomID string) ([]gomatrixserverlib.MSC2946StrippedEvent, error) {
	// don't return any child refs if the room is not a space room
	create := w.stateEvent(roomID, gomatrixserverlib.MRoomCreate, "")
	if create != nil {
		// escape the `.`s so gjson doesn't think it's nested
		roomType := gjson.GetBytes(create.Content(), strings.ReplaceAll(ConstCreateEventContentKey, ".", `\.`)).Str
		if roomType != ConstCreateEventContentValueSpace {
			return nil, nil
		}
	}

	events, err := w.db.ChildReferences(w.ctx, roomID)
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

func parseInt(intstr string, defaultVal int) int {
	i, err := strconv.ParseInt(intstr, 10, 32)
	if err != nil {
		return defaultVal
	}
	return int(i)
}
