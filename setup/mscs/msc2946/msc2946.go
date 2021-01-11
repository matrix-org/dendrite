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
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	chttputil "github.com/matrix-org/dendrite/clientapi/httputil"
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
	createEventContentKey = "org.matrix.msc1772.type"
)

// SpacesRequest is the request body to POST /_matrix/client/r0/rooms/{roomID}/spaces
type SpacesRequest struct {
	MaxRoomsPerSpace int    `json:"max_rooms_per_space"`
	Limit            int    `json:"limit"`
	Batch            string `json:"batch"`
}

// Defaults sets the request defaults
func (r *SpacesRequest) Defaults() {
	r.Limit = 100
	r.MaxRoomsPerSpace = -1
}

// SpacesResponse is the response body to POST /_matrix/client/r0/rooms/{roomID}/spaces
type SpacesResponse struct {
	NextBatch string `json:"next_batch"`
	// Rooms are nodes on the space graph.
	Rooms []Room `json:"rooms"`
	// Events are edges on the space graph, exclusively m.space.child or m.room.parent events
	Events []gomatrixserverlib.ClientEvent `json:"events"`
}

// Room is a node on the space graph
type Room struct {
	gomatrixserverlib.PublicRoom
	NumRefs  int    `json:"num_refs"`
	RoomType string `json:"room_type"`
}

// Enable this MSC
func Enable(
	base *setup.BaseDendrite, rsAPI roomserver.RoomserverInternalAPI, userAPI userapi.UserInternalAPI,
) error {
	db, err := NewDatabase(&base.Cfg.MSCs.Database)
	if err != nil {
		return fmt.Errorf("Cannot enable MSC2946: %w", err)
	}
	hooks.Enable()
	hooks.Attach(hooks.KindNewEventPersisted, func(headeredEvent interface{}) {
		he := headeredEvent.(*gomatrixserverlib.HeaderedEvent)
		hookErr := db.StoreRelation(context.Background(), he)
		if hookErr != nil {
			util.GetLogger(context.Background()).WithError(hookErr).WithField("event_id", he.EventID()).Error(
				"failed to StoreRelation",
			)
		}
	})

	base.PublicClientAPIMux.Handle("/unstable/rooms/{roomID}/spaces",
		httputil.MakeAuthAPI("spaces", userAPI, spacesHandler(db, rsAPI)),
	).Methods(http.MethodPost, http.MethodOptions)
	return nil
}

func spacesHandler(db Database, rsAPI roomserver.RoomserverInternalAPI) func(*http.Request, *userapi.Device) util.JSONResponse {
	inMemoryBatchCache := make(map[string]set)
	return func(req *http.Request, device *userapi.Device) util.JSONResponse {
		// Extract the room ID from the request. Sanity check request data.
		params := mux.Vars(req)
		roomID := params["roomID"]
		var r SpacesRequest
		r.Defaults()
		if resErr := chttputil.UnmarshalJSONRequest(req, &r); resErr != nil {
			return *resErr
		}
		if r.Limit > 100 {
			r.Limit = 100
		}
		w := walker{
			req:        &r,
			rootRoomID: roomID,
			caller:     device,
			ctx:        req.Context(),

			db:                 db,
			rsAPI:              rsAPI,
			inMemoryBatchCache: inMemoryBatchCache,
		}
		res, resErr := w.walk()
		if resErr != nil {
			return *resErr
		}
		return util.JSONResponse{
			Code: 200,
			JSON: res,
		}
	}
}

type walker struct {
	req        *SpacesRequest
	rootRoomID string
	caller     *userapi.Device
	db         Database
	rsAPI      roomserver.RoomserverInternalAPI
	ctx        context.Context

	// user ID|device ID|batch_num => event/room IDs sent to client
	inMemoryBatchCache map[string]set
	mu                 sync.Mutex
}

func (w *walker) alreadySent(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	m, ok := w.inMemoryBatchCache[w.caller.UserID+"|"+w.caller.ID]
	if !ok {
		return false
	}
	return m[id]
}

func (w *walker) markSent(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	m := w.inMemoryBatchCache[w.caller.UserID+"|"+w.caller.ID]
	if m == nil {
		m = make(set)
	}
	m[id] = true
	w.inMemoryBatchCache[w.caller.UserID+"|"+w.caller.ID] = m
}

func (w *walker) walk() (*SpacesResponse, *util.JSONResponse) {
	var res SpacesResponse
	// Begin walking the graph starting with the room ID in the request in a queue of unvisited rooms
	unvisited := []string{w.rootRoomID}
	processed := make(set)
	for len(unvisited) > 0 {
		roomID := unvisited[0]
		unvisited = unvisited[1:]
		// If this room has already been processed, skip. NB: do not remember this between calls
		if processed[roomID] {
			continue
		}
		// Mark this room as processed.
		processed[roomID] = true
		// Is the caller currently joined to the room or is the room `world_readable`
		// If no, skip this room. If yes, continue.
		if !w.authorised(roomID) {
			continue
		}
		// Get all `m.space.child` and `m.room.parent` state events for the room. *In addition*, get
		// all `m.space.child` and `m.room.parent` state events which *point to* (via `state_key` or `content.room_id`)
		// this room. This requires servers to store reverse lookups.
		refs, err := w.references(roomID)
		if err != nil {
			util.GetLogger(w.ctx).WithError(err).WithField("room_id", roomID).Error("failed to extract references for room")
			continue
		}

		// If this room has not ever been in `rooms` (across multiple requests), extract the
		// `PublicRoomsChunk` for this room.
		if !w.alreadySent(roomID) {
			pubRoom := w.publicRoomsChunk(roomID)
			roomType := ""
			create := w.stateEvent(roomID, "m.room.create", "")
			if create != nil {
				roomType = gjson.GetBytes(create.Content(), createEventContentKey).Str
			}

			// Add the total number of events to `PublicRoomsChunk` under `num_refs`. Add `PublicRoomsChunk` to `rooms`.
			res.Rooms = append(res.Rooms, Room{
				PublicRoom: *pubRoom,
				NumRefs:    refs.len(),
				RoomType:   roomType,
			})
		}

		uniqueRooms := make(set)

		// If this is the root room from the original request, insert all these events into `events` if
		// they haven't been added before (across multiple requests).
		if w.rootRoomID == roomID {
			for _, ev := range refs.events() {
				if !w.alreadySent(ev.EventID()) {
					res.Events = append(res.Events, gomatrixserverlib.HeaderedToClientEvent(
						ev, gomatrixserverlib.FormatAll,
					))
					uniqueRooms[ev.RoomID()] = true
					w.markSent(ev.EventID())
				}
			}
		} else {
			// Else add them to `events` honouring the `limit` and `max_rooms_per_space` values. If either
			// are exceeded, stop adding events. If the event has already been added, do not add it again.
			numAdded := 0
			for _, ev := range refs.events() {
				if len(res.Events) >= w.req.Limit {
					break
				}
				if numAdded >= w.req.MaxRoomsPerSpace {
					break
				}
				if w.alreadySent(ev.EventID()) {
					continue
				}
				res.Events = append(res.Events, gomatrixserverlib.HeaderedToClientEvent(
					ev, gomatrixserverlib.FormatAll,
				))
				uniqueRooms[ev.RoomID()] = true
				w.markSent(ev.EventID())
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
	return &res, nil
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
	return nil
}

// authorised returns true iff the user is joined this room or the room is world_readable
func (w *walker) authorised(roomID string) bool {
	return false
}

// references returns all references pointing to or from this room.
func (w *walker) references(roomID string) (eventLookup, error) {
	return nil, nil
}

// state event lookup across multiple rooms keyed on event type
// NOT THREAD SAFE
type eventLookup map[string][]*gomatrixserverlib.HeaderedEvent

func (el eventLookup) get(roomID, evType, stateKey string) *gomatrixserverlib.HeaderedEvent {
	evs := el[evType]
	if len(evs) == 0 {
		return nil
	}
	for _, ev := range evs {
		if ev.RoomID() == roomID && ev.StateKeyEquals(stateKey) {
			return ev
		}
	}
	return nil
}

func (el eventLookup) set(ev *gomatrixserverlib.HeaderedEvent) {
	evs := el[ev.Type()]
	if evs == nil {
		evs = make([]*gomatrixserverlib.HeaderedEvent, 1)
	}
	evs[0] = ev
	el[ev.Type()] = evs
}

func (el eventLookup) len() int {
	sum := 0
	for _, evs := range el {
		sum += len(evs)
	}
	return sum
}

func (el eventLookup) events() (events []*gomatrixserverlib.HeaderedEvent) {
	for _, evs := range el {
		events = append(events, evs...)
	}
	return
}

type set map[string]bool
