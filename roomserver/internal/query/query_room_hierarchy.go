// Copyright 2023 The Matrix.org Foundation C.I.C.
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

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	fs "github.com/matrix-org/dendrite/federationapi/api"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"
)

// Traverse the room hierarchy using the provided walker up to the provided limit,
// returning a new walker which can be used to fetch the next page.
//
// If limit is -1, this is treated as no limit, and the entire hierarchy will be traversed.
//
// If returned walker is nil, then there are no more rooms left to traverse. This method does not modify the provided walker, so it
// can be cached.
func (querier *Queryer) QueryNextRoomHierarchyPage(ctx context.Context, walker roomserver.RoomHierarchyWalker, limit int) ([]fclient.RoomHierarchyRoom, *roomserver.RoomHierarchyWalker, error) {
	if authorised, _ := authorised(ctx, querier, walker.Caller, walker.RootRoomID, nil); !authorised {
		return nil, nil, roomserver.ErrRoomUnknownOrNotAllowed{Err: fmt.Errorf("room is unknown/forbidden")}
	}

	discoveredRooms := []fclient.RoomHierarchyRoom{}

	// Copy unvisited and processed to avoid modifying original walker (which is typically in cache)
	unvisited := make([]roomserver.RoomHierarchyWalkerQueuedRoom, len(walker.Unvisited))
	copy(unvisited, walker.Unvisited)
	processed := walker.Processed.Copy()

	// Depth first -> stack data structure
	for len(unvisited) > 0 {
		if len(discoveredRooms) >= limit && limit != -1 {
			break
		}

		// pop the stack
		queuedRoom := unvisited[len(unvisited)-1]
		unvisited = unvisited[:len(unvisited)-1]
		// If this room has already been processed, skip.
		// If this room exceeds the specified depth, skip.
		if processed.Contains(queuedRoom.RoomID) || (walker.MaxDepth > 0 && queuedRoom.Depth > walker.MaxDepth) {
			continue
		}

		// Mark this room as processed.
		processed.Add(queuedRoom.RoomID)

		// if this room is not a space room, skip.
		var roomType string
		create := stateEvent(ctx, querier, queuedRoom.RoomID, spec.MRoomCreate, "")
		if create != nil {
			var createContent gomatrixserverlib.CreateContent
			err := json.Unmarshal(create.Content(), &createContent)
			if err != nil {
				util.GetLogger(ctx).WithError(err).WithField("create_content", create.Content()).Warn("failed to unmarshal m.room.create event")
			}
			roomType = createContent.RoomType
		}

		// Collect rooms/events to send back (either locally or fetched via federation)
		var discoveredChildEvents []fclient.RoomHierarchyStrippedEvent

		// If we know about this room and the caller is authorised (joined/world_readable) then pull
		// events locally
		roomExists := roomExists(ctx, querier, queuedRoom.RoomID)
		if !roomExists {
			// attempt to query this room over federation, as either we've never heard of it before
			// or we've left it and hence are not authorised (but info may be exposed regardless)
			fedRes := federatedRoomInfo(ctx, querier, walker.Caller, walker.SuggestedOnly, queuedRoom.RoomID, queuedRoom.Vias)
			if fedRes != nil {
				discoveredChildEvents = fedRes.Room.ChildrenState
				discoveredRooms = append(discoveredRooms, fedRes.Room)
				if len(fedRes.Children) > 0 {
					discoveredRooms = append(discoveredRooms, fedRes.Children...)
				}
				// mark this room as a space room as the federated server responded.
				// we need to do this so we add the children of this room to the unvisited stack
				// as these children may be rooms we do know about.
				roomType = spec.MSpace
			}
		} else if authorised, isJoinedOrInvited := authorised(ctx, querier, walker.Caller, queuedRoom.RoomID, queuedRoom.ParentRoomID); authorised {
			// Get all `m.space.child` state events for this room
			events, err := childReferences(ctx, querier, walker.SuggestedOnly, queuedRoom.RoomID)
			if err != nil {
				util.GetLogger(ctx).WithError(err).WithField("room_id", queuedRoom.RoomID).Error("failed to extract references for room")
				continue
			}
			discoveredChildEvents = events

			pubRoom := publicRoomsChunk(ctx, querier, queuedRoom.RoomID)

			discoveredRooms = append(discoveredRooms, fclient.RoomHierarchyRoom{
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
		if roomType != spec.MSpace {
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

			childRoomID, err := spec.NewRoomID(ev.StateKey)

			if err != nil {
				util.GetLogger(ctx).WithError(err).WithField("invalid_room_id", ev.StateKey).WithField("parent_room_id", queuedRoom.RoomID).Warn("Invalid room ID in m.space.child state event")
			} else {
				unvisited = append(unvisited, roomserver.RoomHierarchyWalkerQueuedRoom{
					RoomID:       *childRoomID,
					ParentRoomID: &queuedRoom.RoomID,
					Depth:        queuedRoom.Depth + 1,
					Vias:         spaceContent.Via,
				})
			}
		}
	}

	if len(unvisited) == 0 {
		// If no more rooms to walk, then don't return a walker for future pages
		return discoveredRooms, nil, nil
	} else {
		// If there are more rooms to walk, then return a new walker to resume walking from (for querying more pages)
		newWalker := roomserver.RoomHierarchyWalker{
			RootRoomID:    walker.RootRoomID,
			Caller:        walker.Caller,
			SuggestedOnly: walker.SuggestedOnly,
			MaxDepth:      walker.MaxDepth,
			Unvisited:     unvisited,
			Processed:     processed,
		}

		return discoveredRooms, &newWalker, nil
	}

}

// authorised returns true iff the user is joined this room or the room is world_readable
func authorised(ctx context.Context, querier *Queryer, caller types.DeviceOrServerName, roomID spec.RoomID, parentRoomID *spec.RoomID) (authed, isJoinedOrInvited bool) {
	if clientCaller := caller.Device(); clientCaller != nil {
		return authorisedUser(ctx, querier, clientCaller, roomID, parentRoomID)
	} else {
		return authorisedServer(ctx, querier, roomID, *caller.ServerName()), false
	}
}

// authorisedServer returns true iff the server is joined this room or the room is world_readable, public, or knockable
func authorisedServer(ctx context.Context, querier *Queryer, roomID spec.RoomID, callerServerName spec.ServerName) bool {
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
	err := querier.QueryCurrentState(ctx, &roomserver.QueryCurrentStateRequest{
		RoomID: roomID.String(),
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			hisVisTuple, joinRuleTuple,
		},
	}, &queryRoomRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("failed to QueryCurrentState")
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
	allowJoinedToRoomIDs := []spec.RoomID{roomID}
	joinRuleEv := queryRoomRes.StateEvents[joinRuleTuple]

	if joinRuleEv != nil {
		rule, ruleErr := joinRuleEv.JoinRule()
		if ruleErr != nil {
			util.GetLogger(ctx).WithError(ruleErr).WithField("parent_room_id", roomID).Warn("failed to get join rule")
			return false
		}

		if rule == spec.Public || rule == spec.Knock {
			return true
		}

		if rule == spec.Restricted {
			allowJoinedToRoomIDs = append(allowJoinedToRoomIDs, restrictedJoinRuleAllowedRooms(ctx, joinRuleEv)...)
		}
	}

	// check if server is joined to any allowed room
	for _, allowedRoomID := range allowJoinedToRoomIDs {
		var queryRes fs.QueryJoinedHostServerNamesInRoomResponse
		err = querier.FSAPI.QueryJoinedHostServerNamesInRoom(ctx, &fs.QueryJoinedHostServerNamesInRoomRequest{
			RoomID: allowedRoomID.String(),
		}, &queryRes)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("failed to QueryJoinedHostServerNamesInRoom")
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
func authorisedUser(ctx context.Context, querier *Queryer, clientCaller *userapi.Device, roomID spec.RoomID, parentRoomID *spec.RoomID) (authed bool, isJoinedOrInvited bool) {
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
	err := querier.QueryCurrentState(ctx, &roomserver.QueryCurrentStateRequest{
		RoomID: roomID.String(),
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			hisVisTuple, joinRuleTuple, roomMemberTuple,
		},
	}, &queryRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("failed to QueryCurrentState")
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
	if parentRoomID != nil && joinRuleEv != nil {
		var allowed bool
		rule, ruleErr := joinRuleEv.JoinRule()
		if ruleErr != nil {
			util.GetLogger(ctx).WithError(ruleErr).WithField("parent_room_id", parentRoomID).Warn("failed to get join rule")
		} else if rule == spec.Public || rule == spec.Knock {
			allowed = true
		} else if rule == spec.Restricted {
			allowedRoomIDs := restrictedJoinRuleAllowedRooms(ctx, joinRuleEv)
			// check parent is in the allowed set
			for _, a := range allowedRoomIDs {
				if *parentRoomID == a {
					allowed = true
					break
				}
			}
		}
		if allowed {
			// ensure caller is joined to the parent room
			var queryRes2 roomserver.QueryCurrentStateResponse
			err = querier.QueryCurrentState(ctx, &roomserver.QueryCurrentStateRequest{
				RoomID: parentRoomID.String(),
				StateTuples: []gomatrixserverlib.StateKeyTuple{
					roomMemberTuple,
				},
			}, &queryRes2)
			if err != nil {
				util.GetLogger(ctx).WithError(err).WithField("parent_room_id", parentRoomID).Warn("failed to check user is joined to parent room")
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

// helper function to fetch a state event
func stateEvent(ctx context.Context, querier *Queryer, roomID spec.RoomID, evType, stateKey string) *types.HeaderedEvent {
	var queryRes roomserver.QueryCurrentStateResponse
	tuple := gomatrixserverlib.StateKeyTuple{
		EventType: evType,
		StateKey:  stateKey,
	}
	err := querier.QueryCurrentState(ctx, &roomserver.QueryCurrentStateRequest{
		RoomID:      roomID.String(),
		StateTuples: []gomatrixserverlib.StateKeyTuple{tuple},
	}, &queryRes)
	if err != nil {
		return nil
	}
	return queryRes.StateEvents[tuple]
}

// returns true if the current server is participating in the provided room
func roomExists(ctx context.Context, querier *Queryer, roomID spec.RoomID) bool {
	var queryRes roomserver.QueryServerJoinedToRoomResponse
	err := querier.QueryServerJoinedToRoom(ctx, &roomserver.QueryServerJoinedToRoomRequest{
		RoomID:     roomID.String(),
		ServerName: querier.Cfg.Global.ServerName,
	}, &queryRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("failed to QueryServerJoinedToRoom")
		return false
	}
	// if the room exists but we aren't in the room then we might have stale data so we want to fetch
	// it fresh via federation
	return queryRes.RoomExists && queryRes.IsInRoom
}

// federatedRoomInfo returns more of the spaces graph from another server. Returns nil if this was
// unsuccessful.
func federatedRoomInfo(ctx context.Context, querier *Queryer, caller types.DeviceOrServerName, suggestedOnly bool, roomID spec.RoomID, vias []string) *fclient.RoomHierarchyResponse {
	// only do federated requests for client requests
	if caller.Device() == nil {
		return nil
	}
	resp, ok := querier.Cache.GetRoomHierarchy(roomID.String())
	if ok {
		util.GetLogger(ctx).Debugf("Returning cached response for %s", roomID)
		return &resp
	}
	util.GetLogger(ctx).Debugf("Querying %s via %+v", roomID, vias)
	innerCtx := context.Background()
	// query more of the spaces graph using these servers
	for _, serverName := range vias {
		if serverName == string(querier.Cfg.Global.ServerName) {
			continue
		}
		res, err := querier.FSAPI.RoomHierarchies(innerCtx, querier.Cfg.Global.ServerName, spec.ServerName(serverName), roomID.String(), suggestedOnly)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Warnf("failed to call RoomHierarchies on server %s", serverName)
			continue
		}
		// ensure nil slices are empty as we send this to the client sometimes
		if res.Room.ChildrenState == nil {
			res.Room.ChildrenState = []fclient.RoomHierarchyStrippedEvent{}
		}
		for i := 0; i < len(res.Children); i++ {
			child := res.Children[i]
			if child.ChildrenState == nil {
				child.ChildrenState = []fclient.RoomHierarchyStrippedEvent{}
			}
			res.Children[i] = child
		}
		querier.Cache.StoreRoomHierarchy(roomID.String(), res)

		return &res
	}
	return nil
}

// references returns all child references pointing to or from this room.
func childReferences(ctx context.Context, querier *Queryer, suggestedOnly bool, roomID spec.RoomID) ([]fclient.RoomHierarchyStrippedEvent, error) {
	createTuple := gomatrixserverlib.StateKeyTuple{
		EventType: spec.MRoomCreate,
		StateKey:  "",
	}
	var res roomserver.QueryCurrentStateResponse
	err := querier.QueryCurrentState(context.Background(), &roomserver.QueryCurrentStateRequest{
		RoomID:         roomID.String(),
		AllowWildcards: true,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			createTuple, {
				EventType: spec.MSpaceChild,
				StateKey:  "*",
			},
		},
	}, &res)
	if err != nil {
		return nil, err
	}

	// don't return any child refs if the room is not a space room
	if create := res.StateEvents[createTuple]; create != nil {
		var createContent gomatrixserverlib.CreateContent
		err := json.Unmarshal(create.Content(), &createContent)
		if err != nil {
			util.GetLogger(ctx).WithError(err).WithField("create_content", create.Content()).Warn("failed to unmarshal m.room.create event")
		}
		roomType := createContent.RoomType
		if roomType != spec.MSpace {
			return []fclient.RoomHierarchyStrippedEvent{}, nil
		}
	}
	delete(res.StateEvents, createTuple)

	el := make([]fclient.RoomHierarchyStrippedEvent, 0, len(res.StateEvents))
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
			if suggestedOnly && !content.Get("suggested").Bool() {
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

// fetch public room information for provided room
func publicRoomsChunk(ctx context.Context, querier *Queryer, roomID spec.RoomID) *fclient.PublicRoom {
	pubRooms, err := roomserver.PopulatePublicRooms(ctx, []string{roomID.String()}, querier)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("failed to PopulatePublicRooms")
		return nil
	}
	if len(pubRooms) == 0 {
		return nil
	}
	return &pubRooms[0]
}

func stripped(ev gomatrixserverlib.PDU) *fclient.RoomHierarchyStrippedEvent {
	if ev.StateKey() == nil {
		return nil
	}
	return &fclient.RoomHierarchyStrippedEvent{
		Type:           ev.Type(),
		StateKey:       *ev.StateKey(),
		Content:        ev.Content(),
		Sender:         string(ev.SenderID()),
		OriginServerTS: ev.OriginServerTS(),
	}
}

// given join_rule event, return list of rooms where membership of that room allows joining.
func restrictedJoinRuleAllowedRooms(ctx context.Context, joinRuleEv *types.HeaderedEvent) (allows []spec.RoomID) {
	rule, _ := joinRuleEv.JoinRule()
	if rule != spec.Restricted {
		return nil
	}
	var jrContent gomatrixserverlib.JoinRuleContent
	if err := json.Unmarshal(joinRuleEv.Content(), &jrContent); err != nil {
		util.GetLogger(ctx).Warnf("failed to check join_rule on room %s: %s", joinRuleEv.RoomID(), err)
		return nil
	}
	for _, allow := range jrContent.Allow {
		if allow.Type == spec.MRoomMembership {
			allowedRoomID, err := spec.NewRoomID(allow.RoomID)
			if err != nil {
				util.GetLogger(ctx).Warnf("invalid room ID '%s' found in join_rule on room %s: %s", allow.RoomID, joinRuleEv.RoomID(), err)
			} else {
				allows = append(allows, *allowedRoomID)
			}
		}
	}
	return
}
