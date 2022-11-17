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
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	fs "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/caching"
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
	NextBatch string                          `json:"next_batch,omitempty"`
}

// Enable this MSC
func Enable(
	base *base.BaseDendrite, rsAPI roomserver.RoomserverInternalAPI, userAPI userapi.UserInternalAPI,
	fsAPI fs.FederationInternalAPI, keyRing gomatrixserverlib.JSONVerifier, cache caching.SpaceSummaryRoomsCache,
) error {
	clientAPI := httputil.MakeAuthAPI("spaces", userAPI, spacesHandler(rsAPI, fsAPI, cache, base.Cfg.Global.ServerName), httputil.WithAllowGuests())
	base.PublicClientAPIMux.Handle("/v1/rooms/{roomID}/hierarchy", clientAPI).Methods(http.MethodGet, http.MethodOptions)
	base.PublicClientAPIMux.Handle("/unstable/org.matrix.msc2946/rooms/{roomID}/hierarchy", clientAPI).Methods(http.MethodGet, http.MethodOptions)

	fedAPI := httputil.MakeExternalAPI(
		"msc2946_fed_spaces", func(req *http.Request) util.JSONResponse {
			fedReq, errResp := gomatrixserverlib.VerifyHTTPRequest(
				req, time.Now(), base.Cfg.Global.ServerName, base.Cfg.Global.IsLocalServerName, keyRing,
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
			return federatedSpacesHandler(req.Context(), fedReq, roomID, cache, rsAPI, fsAPI, base.Cfg.Global.ServerName)
		},
	)
	base.PublicFederationAPIMux.Handle("/unstable/org.matrix.msc2946/hierarchy/{roomID}", fedAPI).Methods(http.MethodGet)
	base.PublicFederationAPIMux.Handle("/v1/hierarchy/{roomID}", fedAPI).Methods(http.MethodGet)
	return nil
}

func federatedSpacesHandler(
	ctx context.Context, fedReq *gomatrixserverlib.FederationRequest, roomID string,
	cache caching.SpaceSummaryRoomsCache,
	rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationInternalAPI,
	thisServer gomatrixserverlib.ServerName,
) util.JSONResponse {
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
		cache:         cache,
		suggestedOnly: u.Query().Get("suggested_only") == "true",
		limit:         1000,
		// The main difference is that it does not recurse into spaces and does not support pagination.
		// This is somewhat equivalent to a Client-Server request with a max_depth=1.
		maxDepth: 1,

		rsAPI: rsAPI,
		fsAPI: fsAPI,
		// inline cache as we don't have pagination in federation mode
		paginationCache: make(map[string]paginationInfo),
	}
	return w.walk()
}

func spacesHandler(
	rsAPI roomserver.RoomserverInternalAPI,
	fsAPI fs.FederationInternalAPI,
	cache caching.SpaceSummaryRoomsCache,
	thisServer gomatrixserverlib.ServerName,
) func(*http.Request, *userapi.Device) util.JSONResponse {
	// declared outside the returned handler so it persists between calls
	// TODO: clear based on... time?
	paginationCache := make(map[string]paginationInfo)

	return func(req *http.Request, device *userapi.Device) util.JSONResponse {
		// Extract the room ID from the request. Sanity check request data.
		params, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		roomID := params["roomID"]
		w := walker{
			suggestedOnly:   req.URL.Query().Get("suggested_only") == "true",
			limit:           parseInt(req.URL.Query().Get("limit"), 1000),
			maxDepth:        parseInt(req.URL.Query().Get("max_depth"), -1),
			paginationToken: req.URL.Query().Get("from"),
			rootRoomID:      roomID,
			caller:          device,
			thisServer:      thisServer,
			ctx:             req.Context(),
			cache:           cache,

			rsAPI:           rsAPI,
			fsAPI:           fsAPI,
			paginationCache: paginationCache,
		}
		return w.walk()
	}
}

type paginationInfo struct {
	processed set
	unvisited []roomVisit
}

type walker struct {
	rootRoomID      string
	caller          *userapi.Device
	serverName      gomatrixserverlib.ServerName
	thisServer      gomatrixserverlib.ServerName
	rsAPI           roomserver.RoomserverInternalAPI
	fsAPI           fs.FederationInternalAPI
	ctx             context.Context
	cache           caching.SpaceSummaryRoomsCache
	suggestedOnly   bool
	limit           int
	maxDepth        int
	paginationToken string

	paginationCache map[string]paginationInfo
	mu              sync.Mutex
}

func (w *walker) newPaginationCache() (string, paginationInfo) {
	p := paginationInfo{
		processed: make(set),
		unvisited: nil,
	}
	tok := uuid.NewString()
	return tok, p
}

func (w *walker) loadPaginationCache(paginationToken string) *paginationInfo {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.paginationCache[paginationToken]
	return &p
}

func (w *walker) storePaginationCache(paginationToken string, cache paginationInfo) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.paginationCache[paginationToken] = cache
}

type roomVisit struct {
	roomID       string
	parentRoomID string
	depth        int
	vias         []string // vias to query this room by
}

func (w *walker) walk() util.JSONResponse {
	if authorised, _ := w.authorised(w.rootRoomID, ""); !authorised {
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

	var cache *paginationInfo
	if w.paginationToken != "" {
		cache = w.loadPaginationCache(w.paginationToken)
		if cache == nil {
			return util.JSONResponse{
				Code: 400,
				JSON: jsonerror.InvalidArgumentValue("invalid from"),
			}
		}
	} else {
		tok, c := w.newPaginationCache()
		cache = &c
		w.paginationToken = tok
		// Begin walking the graph starting with the room ID in the request in a queue of unvisited rooms
		c.unvisited = append(c.unvisited, roomVisit{
			roomID:       w.rootRoomID,
			parentRoomID: "",
			depth:        0,
		})
	}

	processed := cache.processed
	unvisited := cache.unvisited

	// Depth first -> stack data structure
	for len(unvisited) > 0 {
		if len(discoveredRooms) >= w.limit {
			break
		}

		// pop the stack
		rv := unvisited[len(unvisited)-1]
		unvisited = unvisited[:len(unvisited)-1]
		// If this room has already been processed, skip.
		// If this room exceeds the specified depth, skip.
		if processed.isSet(rv.roomID) || rv.roomID == "" || (w.maxDepth > 0 && rv.depth > w.maxDepth) {
			continue
		}

		// Mark this room as processed.
		processed.set(rv.roomID)

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
		roomExists := w.roomExists(rv.roomID)
		if !roomExists {
			// attempt to query this room over federation, as either we've never heard of it before
			// or we've left it and hence are not authorised (but info may be exposed regardless)
			fedRes := w.federatedRoomInfo(rv.roomID, rv.vias)
			if fedRes != nil {
				discoveredChildEvents = fedRes.Room.ChildrenState
				discoveredRooms = append(discoveredRooms, fedRes.Room)
				if len(fedRes.Children) > 0 {
					discoveredRooms = append(discoveredRooms, fedRes.Children...)
				}
				// mark this room as a space room as the federated server responded.
				// we need to do this so we add the children of this room to the unvisited stack
				// as these children may be rooms we do know about.
				roomType = ConstCreateEventContentValueSpace
			}
		} else if authorised, isJoinedOrInvited := w.authorised(rv.roomID, rv.parentRoomID); authorised {
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
			// don't walk children if the user is not joined/invited to the space
			if !isJoinedOrInvited {
				continue
			}
		} else {
			// room exists but user is not authorised
			continue
		}

		// don't walk the children
		// if the parent is not a space room
		if roomType != ConstCreateEventContentValueSpace {
			continue
		}

		// For each referenced room ID in the child events being returned to the caller
		// add the room ID to the queue of unvisited rooms. Loop from the beginning.
		// We need to invert the order here because the child events are lo->hi on the timestamp,
		// so we need to ensure we pop in the same lo->hi order, which won't be the case if we
		// insert the highest timestamp last in a stack.
		for i := len(discoveredChildEvents) - 1; i >= 0; i-- {
			spaceContent := struct {
				Via []string `json:"via"`
			}{}
			ev := discoveredChildEvents[i]
			_ = json.Unmarshal(ev.Content, &spaceContent)
			unvisited = append(unvisited, roomVisit{
				roomID:       ev.StateKey,
				parentRoomID: rv.roomID,
				depth:        rv.depth + 1,
				vias:         spaceContent.Via,
			})
		}
	}

	if len(unvisited) > 0 {
		// we still have more rooms so we need to send back a pagination token,
		// we probably hit a room limit
		cache.processed = processed
		cache.unvisited = unvisited
		w.storePaginationCache(w.paginationToken, *cache)
	} else {
		// clear the pagination token so we don't send it back to the client
		// Note we do NOT nuke the cache just in case this response is lost
		// and the client retries it.
		w.paginationToken = ""
	}

	if w.caller != nil {
		// return CS API format
		return util.JSONResponse{
			Code: 200,
			JSON: MSC2946ClientResponse{
				Rooms:     discoveredRooms,
				NextBatch: w.paginationToken,
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
func (w *walker) federatedRoomInfo(roomID string, vias []string) *gomatrixserverlib.MSC2946SpacesResponse {
	// only do federated requests for client requests
	if w.caller == nil {
		return nil
	}
	resp, ok := w.cache.GetSpaceSummary(roomID)
	if ok {
		util.GetLogger(w.ctx).Debugf("Returning cached response for %s", roomID)
		return &resp
	}
	util.GetLogger(w.ctx).Debugf("Querying %s via %+v", roomID, vias)
	ctx := context.Background()
	// query more of the spaces graph using these servers
	for _, serverName := range vias {
		if serverName == string(w.thisServer) {
			continue
		}
		res, err := w.fsAPI.MSC2946Spaces(ctx, w.thisServer, gomatrixserverlib.ServerName(serverName), roomID, w.suggestedOnly)
		if err != nil {
			util.GetLogger(w.ctx).WithError(err).Warnf("failed to call MSC2946Spaces on server %s", serverName)
			continue
		}
		// ensure nil slices are empty as we send this to the client sometimes
		if res.Room.ChildrenState == nil {
			res.Room.ChildrenState = []gomatrixserverlib.MSC2946StrippedEvent{}
		}
		for i := 0; i < len(res.Children); i++ {
			child := res.Children[i]
			if child.ChildrenState == nil {
				child.ChildrenState = []gomatrixserverlib.MSC2946StrippedEvent{}
			}
			res.Children[i] = child
		}
		w.cache.StoreSpaceSummary(roomID, res)

		return &res
	}
	return nil
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
func (w *walker) authorised(roomID, parentRoomID string) (authed, isJoinedOrInvited bool) {
	if w.caller != nil {
		return w.authorisedUser(roomID, parentRoomID)
	}
	return w.authorisedServer(roomID), false
}

// authorisedServer returns true iff the server is joined this room or the room is world_readable, public, or knockable
func (w *walker) authorisedServer(roomID string) bool {
	// Check history visibility / join rules first
	hisVisTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomHistoryVisibility,
		StateKey:  "",
	}
	joinRuleTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomJoinRules,
		StateKey:  "",
	}
	var queryRoomRes roomserver.QueryCurrentStateResponse
	err := w.rsAPI.QueryCurrentState(w.ctx, &roomserver.QueryCurrentStateRequest{
		RoomID: roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			hisVisTuple, joinRuleTuple,
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

	// check if this room is a restricted room and if so, we need to check if the server is joined to an allowed room ID
	// in addition to the actual room ID (but always do the actual one first as it's quicker in the common case)
	allowJoinedToRoomIDs := []string{roomID}
	joinRuleEv := queryRoomRes.StateEvents[joinRuleTuple]

	if joinRuleEv != nil {
		rule, ruleErr := joinRuleEv.JoinRule()
		if ruleErr != nil {
			util.GetLogger(w.ctx).WithError(ruleErr).WithField("parent_room_id", roomID).Warn("failed to get join rule")
			return false
		}

		if rule == gomatrixserverlib.Public || rule == gomatrixserverlib.Knock {
			return true
		}

		if rule == gomatrixserverlib.Restricted {
			allowJoinedToRoomIDs = append(allowJoinedToRoomIDs, w.restrictedJoinRuleAllowedRooms(joinRuleEv, "m.room_membership")...)
		}
	}

	// check if server is joined to any allowed room
	for _, allowedRoomID := range allowJoinedToRoomIDs {
		var queryRes fs.QueryJoinedHostServerNamesInRoomResponse
		err = w.fsAPI.QueryJoinedHostServerNamesInRoom(w.ctx, &fs.QueryJoinedHostServerNamesInRoomRequest{
			RoomID: allowedRoomID,
		}, &queryRes)
		if err != nil {
			util.GetLogger(w.ctx).WithError(err).Error("failed to QueryJoinedHostServerNamesInRoom")
			continue
		}
		for _, srv := range queryRes.ServerNames {
			if srv == w.serverName {
				return true
			}
		}
	}

	return false
}

// authorisedUser returns true iff the user is invited/joined this room or the room is world_readable
// or if the room has a public or knock join rule.
// Failing that, if the room has a restricted join rule and belongs to the space parent listed, it will return true.
func (w *walker) authorisedUser(roomID, parentRoomID string) (authed bool, isJoinedOrInvited bool) {
	hisVisTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomHistoryVisibility,
		StateKey:  "",
	}
	joinRuleTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomJoinRules,
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
			hisVisTuple, joinRuleTuple, roomMemberTuple,
		},
	}, &queryRes)
	if err != nil {
		util.GetLogger(w.ctx).WithError(err).Error("failed to QueryCurrentState")
		return false, false
	}
	memberEv := queryRes.StateEvents[roomMemberTuple]
	if memberEv != nil {
		membership, _ := memberEv.Membership()
		if membership == gomatrixserverlib.Join || membership == gomatrixserverlib.Invite {
			return true, true
		}
	}
	hisVisEv := queryRes.StateEvents[hisVisTuple]
	if hisVisEv != nil {
		hisVis, _ := hisVisEv.HistoryVisibility()
		if hisVis == "world_readable" {
			return true, false
		}
	}
	joinRuleEv := queryRes.StateEvents[joinRuleTuple]
	if parentRoomID != "" && joinRuleEv != nil {
		var allowed bool
		rule, ruleErr := joinRuleEv.JoinRule()
		if ruleErr != nil {
			util.GetLogger(w.ctx).WithError(ruleErr).WithField("parent_room_id", parentRoomID).Warn("failed to get join rule")
		} else if rule == gomatrixserverlib.Public || rule == gomatrixserverlib.Knock {
			allowed = true
		} else if rule == gomatrixserverlib.Restricted {
			allowedRoomIDs := w.restrictedJoinRuleAllowedRooms(joinRuleEv, "m.room_membership")
			// check parent is in the allowed set
			for _, a := range allowedRoomIDs {
				if parentRoomID == a {
					allowed = true
					break
				}
			}
		}
		if allowed {
			// ensure caller is joined to the parent room
			var queryRes2 roomserver.QueryCurrentStateResponse
			err = w.rsAPI.QueryCurrentState(w.ctx, &roomserver.QueryCurrentStateRequest{
				RoomID: parentRoomID,
				StateTuples: []gomatrixserverlib.StateKeyTuple{
					roomMemberTuple,
				},
			}, &queryRes2)
			if err != nil {
				util.GetLogger(w.ctx).WithError(err).WithField("parent_room_id", parentRoomID).Warn("failed to check user is joined to parent room")
			} else {
				memberEv = queryRes2.StateEvents[roomMemberTuple]
				if memberEv != nil {
					membership, _ := memberEv.Membership()
					if membership == gomatrixserverlib.Join {
						return true, false
					}
				}
			}
		}
	}
	return false, false
}

func (w *walker) restrictedJoinRuleAllowedRooms(joinRuleEv *gomatrixserverlib.HeaderedEvent, allowType string) (allows []string) {
	rule, _ := joinRuleEv.JoinRule()
	if rule != gomatrixserverlib.Restricted {
		return nil
	}
	var jrContent gomatrixserverlib.JoinRuleContent
	if err := json.Unmarshal(joinRuleEv.Content(), &jrContent); err != nil {
		util.GetLogger(w.ctx).Warnf("failed to check join_rule on room %s: %s", joinRuleEv.RoomID(), err)
		return nil
	}
	for _, allow := range jrContent.Allow {
		if allow.Type == allowType {
			allows = append(allows, allow.RoomID)
		}
	}
	return
}

// references returns all child references pointing to or from this room.
func (w *walker) childReferences(roomID string) ([]gomatrixserverlib.MSC2946StrippedEvent, error) {
	createTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomCreate,
		StateKey:  "",
	}
	var res roomserver.QueryCurrentStateResponse
	err := w.rsAPI.QueryCurrentState(context.Background(), &roomserver.QueryCurrentStateRequest{
		RoomID:         roomID,
		AllowWildcards: true,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			createTuple, {
				EventType: ConstSpaceChildEventType,
				StateKey:  "*",
			},
		},
	}, &res)
	if err != nil {
		return nil, err
	}

	// don't return any child refs if the room is not a space room
	if res.StateEvents[createTuple] != nil {
		// escape the `.`s so gjson doesn't think it's nested
		roomType := gjson.GetBytes(res.StateEvents[createTuple].Content(), strings.ReplaceAll(ConstCreateEventContentKey, ".", `\.`)).Str
		if roomType != ConstCreateEventContentValueSpace {
			return []gomatrixserverlib.MSC2946StrippedEvent{}, nil
		}
	}
	delete(res.StateEvents, createTuple)

	el := make([]gomatrixserverlib.MSC2946StrippedEvent, 0, len(res.StateEvents))
	for _, ev := range res.StateEvents {
		content := gjson.ParseBytes(ev.Content())
		// only return events that have a `via` key as per MSC1772
		// else we'll incorrectly walk redacted events (as the link
		// is in the state_key)
		if content.Get("via").Exists() {
			strip := stripped(ev.Event)
			if strip == nil {
				continue
			}
			// if suggested only and this child isn't suggested, skip it.
			// if suggested only = false we include everything so don't need to check the content.
			if w.suggestedOnly && !content.Get("suggested").Bool() {
				continue
			}
			el = append(el, *strip)
		}
	}
	// sort by origin_server_ts as per MSC2946
	sort.Slice(el, func(i, j int) bool {
		return el[i].OriginServerTS < el[j].OriginServerTS
	})

	return el, nil
}

type set map[string]struct{}

func (s set) set(val string) {
	s[val] = struct{}{}
}
func (s set) isSet(val string) bool {
	_, ok := s[val]
	return ok
}

func stripped(ev *gomatrixserverlib.Event) *gomatrixserverlib.MSC2946StrippedEvent {
	if ev.StateKey() == nil {
		return nil
	}
	return &gomatrixserverlib.MSC2946StrippedEvent{
		Type:           ev.Type(),
		StateKey:       *ev.StateKey(),
		Content:        ev.Content(),
		Sender:         ev.Sender(),
		OriginServerTS: ev.OriginServerTS(),
	}
}

func parseInt(intstr string, defaultVal int) int {
	i, err := strconv.ParseInt(intstr, 10, 32)
	if err != nil {
		return defaultVal
	}
	return int(i)
}
