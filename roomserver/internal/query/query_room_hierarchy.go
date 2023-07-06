package query

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	fs "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/caching"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"
)

// Query the hierarchy of a room (A.K.A 'space summary')
//
// This function returns a iterator-like struct over the hierarchy of a room.
func (r *Queryer) QueryRoomHierarchy(ctx context.Context, caller types.DeviceOrServerName, roomID spec.RoomID, suggestedOnly bool, maxDepth int) roomserver.RoomHierarchyWalker {
	walker := RoomHierarchyWalker{
		rootRoomID:         roomID.String(),
		caller:             caller,
		thisServer:         r.Cfg.Global.ServerName,
		rsAPI:              r,
		fsAPI:              r.FSAPI,
		ctx:                ctx,
		roomHierarchyCache: r.Cache,
		suggestedOnly:      suggestedOnly,
		maxDepth:           maxDepth,
		unvisited: []roomVisit{{
			roomID:       roomID.String(),
			parentRoomID: "",
			depth:        0,
		}},
	}

	return &walker
}

type stringSet map[string]struct{}

func (s stringSet) contains(val string) bool {
	_, ok := s[val]
	return ok
}

func (s stringSet) add(val string) {
	s[val] = struct{}{}
}

type RoomHierarchyWalker struct {
	rootRoomID         string // TODO change to spec.RoomID
	caller             types.DeviceOrServerName
	thisServer         spec.ServerName
	rsAPI              *Queryer
	fsAPI              fs.RoomserverFederationAPI
	ctx                context.Context
	roomHierarchyCache caching.RoomHierarchyCache
	suggestedOnly      bool
	maxDepth           int

	processed stringSet
	unvisited []roomVisit

	done bool
}

const (
	ConstCreateEventContentKey        = "type"
	ConstCreateEventContentValueSpace = "m.space"
	ConstSpaceChildEventType          = "m.space.child"
	ConstSpaceParentEventType         = "m.space.parent"
)

func (w *RoomHierarchyWalker) NextPage(limit int) ([]fclient.MSC2946Room, error) {
	if authorised, _ := w.authorised(w.rootRoomID, ""); !authorised {
		return nil, spec.Forbidden("room is unknown/forbidden")
	}

	var discoveredRooms []fclient.MSC2946Room

	// Depth first -> stack data structure
	for len(w.unvisited) > 0 {
		if len(discoveredRooms) >= limit {
			break
		}

		// pop the stack
		rv := w.unvisited[len(w.unvisited)-1]
		w.unvisited = w.unvisited[:len(w.unvisited)-1]
		// If this room has already been processed, skip.
		// If this room exceeds the specified depth, skip.
		if w.processed.contains(rv.roomID) || rv.roomID == "" || (w.maxDepth > 0 && rv.depth > w.maxDepth) {
			continue
		}

		// Mark this room as processed.
		w.processed.add(rv.roomID)

		// if this room is not a space room, skip.
		var roomType string
		create := w.stateEvent(rv.roomID, spec.MRoomCreate, "")
		if create != nil {
			// escape the `.`s so gjson doesn't think it's nested
			roomType = gjson.GetBytes(create.Content(), strings.ReplaceAll(ConstCreateEventContentKey, ".", `\.`)).Str
		}

		// Collect rooms/events to send back (either locally or fetched via federation)
		var discoveredChildEvents []fclient.MSC2946StrippedEvent

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

			discoveredRooms = append(discoveredRooms, fclient.MSC2946Room{
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
			w.unvisited = append(w.unvisited, roomVisit{
				roomID:       ev.StateKey,
				parentRoomID: rv.roomID,
				depth:        rv.depth + 1,
				vias:         spaceContent.Via,
			})
		}
	}

	if len(w.unvisited) == 0 {
		w.done = true
	}

	return discoveredRooms, nil
}

func (w *RoomHierarchyWalker) Done() bool {
	return w.done
}

func (w *RoomHierarchyWalker) GetCached() roomserver.CachedRoomHierarchyWalker {
	return CachedRoomHierarchyWalker{
		rootRoomID:    w.rootRoomID,
		caller:        w.caller,
		thisServer:    w.thisServer,
		rsAPI:         w.rsAPI,
		fsAPI:         w.fsAPI,
		ctx:           w.ctx,
		cache:         w.roomHierarchyCache,
		suggestedOnly: w.suggestedOnly,
		maxDepth:      w.maxDepth,
		processed:     w.processed,
		unvisited:     w.unvisited,
		done:          w.done,
	}
}

func (w *RoomHierarchyWalker) stateEvent(roomID, evType, stateKey string) *types.HeaderedEvent {
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

func (w *RoomHierarchyWalker) roomExists(roomID string) bool {
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

// federatedRoomInfo returns more of the spaces graph from another server. Returns nil if this was
// unsuccessful.
func (w *RoomHierarchyWalker) federatedRoomInfo(roomID string, vias []string) *fclient.MSC2946SpacesResponse {
	// only do federated requests for client requests
	if w.caller.Device() == nil {
		return nil
	}
	resp, ok := w.roomHierarchyCache.GetRoomHierarchy(roomID)
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
		res, err := w.fsAPI.RoomHierarchies(ctx, w.thisServer, spec.ServerName(serverName), roomID, w.suggestedOnly)
		if err != nil {
			util.GetLogger(w.ctx).WithError(err).Warnf("failed to call MSC2946Spaces on server %s", serverName)
			continue
		}
		// ensure nil slices are empty as we send this to the client sometimes
		if res.Room.ChildrenState == nil {
			res.Room.ChildrenState = []fclient.MSC2946StrippedEvent{}
		}
		for i := 0; i < len(res.Children); i++ {
			child := res.Children[i]
			if child.ChildrenState == nil {
				child.ChildrenState = []fclient.MSC2946StrippedEvent{}
			}
			res.Children[i] = child
		}
		w.roomHierarchyCache.StoreRoomHierarchy(roomID, res)

		return &res
	}
	return nil
}

// references returns all child references pointing to or from this room.
func (w *RoomHierarchyWalker) childReferences(roomID string) ([]fclient.MSC2946StrippedEvent, error) {
	createTuple := gomatrixserverlib.StateKeyTuple{
		EventType: spec.MRoomCreate,
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
			return []fclient.MSC2946StrippedEvent{}, nil
		}
	}
	delete(res.StateEvents, createTuple)

	el := make([]fclient.MSC2946StrippedEvent, 0, len(res.StateEvents))
	for _, ev := range res.StateEvents {
		content := gjson.ParseBytes(ev.Content())
		// only return events that have a `via` key as per MSC1772
		// else we'll incorrectly walk redacted events (as the link
		// is in the state_key)
		if content.Get("via").Exists() {
			strip := stripped(ev.PDU)
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

// authorised returns true iff the user is joined this room or the room is world_readable
func (w *RoomHierarchyWalker) authorised(roomID, parentRoomID string) (authed, isJoinedOrInvited bool) {
	if clientCaller := w.caller.Device(); clientCaller != nil {
		return w.authorisedUser(roomID, clientCaller, parentRoomID)
	} else {
		return w.authorisedServer(roomID, *w.caller.ServerName()), false
	}
}

// authorisedServer returns true iff the server is joined this room or the room is world_readable, public, or knockable
func (w *RoomHierarchyWalker) authorisedServer(roomID string, callerServerName spec.ServerName) bool {
	// Check history visibility / join rules first
	hisVisTuple := gomatrixserverlib.StateKeyTuple{
		EventType: spec.MRoomHistoryVisibility,
		StateKey:  "",
	}
	joinRuleTuple := gomatrixserverlib.StateKeyTuple{
		EventType: spec.MRoomJoinRules,
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

		if rule == spec.Public || rule == spec.Knock {
			return true
		}

		if rule == spec.Restricted {
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
			if srv == callerServerName {
				return true
			}
		}
	}

	return false
}

// authorisedUser returns true iff the user is invited/joined this room or the room is world_readable
// or if the room has a public or knock join rule.
// Failing that, if the room has a restricted join rule and belongs to the space parent listed, it will return true.
func (w *RoomHierarchyWalker) authorisedUser(roomID string, clientCaller *userapi.Device, parentRoomID string) (authed bool, isJoinedOrInvited bool) {
	hisVisTuple := gomatrixserverlib.StateKeyTuple{
		EventType: spec.MRoomHistoryVisibility,
		StateKey:  "",
	}
	joinRuleTuple := gomatrixserverlib.StateKeyTuple{
		EventType: spec.MRoomJoinRules,
		StateKey:  "",
	}
	roomMemberTuple := gomatrixserverlib.StateKeyTuple{
		EventType: spec.MRoomMember,
		StateKey:  clientCaller.UserID,
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
		if membership == spec.Join || membership == spec.Invite {
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
		} else if rule == spec.Public || rule == spec.Knock {
			allowed = true
		} else if rule == spec.Restricted {
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
					if membership == spec.Join {
						return true, false
					}
				}
			}
		}
	}
	return false, false
}

func (w *RoomHierarchyWalker) publicRoomsChunk(roomID string) *fclient.PublicRoom {
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

type roomVisit struct {
	roomID       string
	parentRoomID string
	depth        int
	vias         []string // vias to query this room by
}

func stripped(ev gomatrixserverlib.PDU) *fclient.MSC2946StrippedEvent {
	if ev.StateKey() == nil {
		return nil
	}
	return &fclient.MSC2946StrippedEvent{
		Type:           ev.Type(),
		StateKey:       *ev.StateKey(),
		Content:        ev.Content(),
		Sender:         string(ev.SenderID()),
		OriginServerTS: ev.OriginServerTS(),
	}
}

func (w *RoomHierarchyWalker) restrictedJoinRuleAllowedRooms(joinRuleEv *types.HeaderedEvent, allowType string) (allows []string) {
	rule, _ := joinRuleEv.JoinRule()
	if rule != spec.Restricted {
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

// Stripped down version of RoomHierarchyWalker suitable for caching (For pagination purposes)
//
// TODO remove more stuff
type CachedRoomHierarchyWalker struct {
	rootRoomID    string
	caller        types.DeviceOrServerName
	thisServer    spec.ServerName
	rsAPI         *Queryer
	fsAPI         fs.RoomserverFederationAPI
	ctx           context.Context
	cache         caching.RoomHierarchyCache
	suggestedOnly bool
	maxDepth      int

	processed stringSet
	unvisited []roomVisit

	done bool
}

func (c CachedRoomHierarchyWalker) GetWalker() roomserver.RoomHierarchyWalker {
	return &RoomHierarchyWalker{
		rootRoomID:         c.rootRoomID,
		caller:             c.caller,
		thisServer:         c.thisServer,
		rsAPI:              c.rsAPI,
		fsAPI:              c.fsAPI,
		ctx:                c.ctx,
		roomHierarchyCache: c.cache,
		suggestedOnly:      c.suggestedOnly,
		maxDepth:           c.maxDepth,
		processed:          c.processed,
		unvisited:          c.unvisited,
		done:               c.done,
	}
}

func (c CachedRoomHierarchyWalker) ValidateParams(suggestedOnly bool, maxDepth int) bool {
	return c.suggestedOnly == suggestedOnly && c.maxDepth == maxDepth
}
