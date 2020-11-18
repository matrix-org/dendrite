// Copyright 2020 The Matrix.org Foundation C.I.C.
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

// Package msc2836 'Threading' implements https://github.com/matrix-org/matrix-doc/pull/2836
package msc2836

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	fs "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/setup"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const (
	constRelType     = "m.reference"
	constRoomIDKey   = "relationship_room_id"
	constRoomServers = "relationship_servers"
)

type EventRelationshipRequest struct {
	EventID         string `json:"event_id"`
	MaxDepth        int    `json:"max_depth"`
	MaxBreadth      int    `json:"max_breadth"`
	Limit           int    `json:"limit"`
	DepthFirst      *bool  `json:"depth_first"`
	RecentFirst     *bool  `json:"recent_first"`
	IncludeParent   *bool  `json:"include_parent"`
	IncludeChildren *bool  `json:"include_children"`
	Direction       string `json:"direction"`
	Batch           string `json:"batch"`
	AutoJoin        bool   `json:"auto_join"`
}

func (r *EventRelationshipRequest) applyDefaults() {
	if r.Limit > 100 || r.Limit < 1 {
		r.Limit = 100
	}
	if r.MaxBreadth == 0 {
		r.MaxBreadth = 10
	}
	if r.MaxDepth == 0 {
		r.MaxDepth = 3
	}
	t := true
	f := false
	if r.DepthFirst == nil {
		r.DepthFirst = &f
	}
	if r.RecentFirst == nil {
		r.RecentFirst = &t
	}
	if r.IncludeParent == nil {
		r.IncludeParent = &f
	}
	if r.IncludeChildren == nil {
		r.IncludeChildren = &f
	}
	if r.Direction != "up" {
		r.Direction = "down"
	}
}

type EventRelationshipResponse struct {
	Events    []gomatrixserverlib.ClientEvent `json:"events"`
	NextBatch string                          `json:"next_batch"`
	Limited   bool                            `json:"limited"`
}

// Enable this MSC
func Enable(base *setup.BaseDendrite, rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationSenderInternalAPI, userAPI userapi.UserInternalAPI) error {
	db, err := NewDatabase(&base.Cfg.MSCs.Database)
	if err != nil {
		return fmt.Errorf("Cannot enable MSC2836: %w", err)
	}
	hooks.Enable()
	hooks.Attach(hooks.KindNewEvent, func(headeredEvent interface{}) {
		he := headeredEvent.(*gomatrixserverlib.HeaderedEvent)
		hookErr := db.StoreRelation(context.Background(), he)
		if hookErr != nil {
			util.GetLogger(context.Background()).WithError(hookErr).Error(
				"failed to StoreRelation",
			)
		}
	})
	hooks.Attach(hooks.KindModifyNewEvent, func(headeredEvent interface{}) {
		he := headeredEvent.(*gomatrixserverlib.HeaderedEvent)
		ctx := context.Background()
		// we only inject metadata for events our server sends
		userID := he.Sender()
		_, domain, err := gomatrixserverlib.SplitID('@', userID)
		if err != nil {
			return
		}
		if domain != base.Cfg.Global.ServerName {
			return
		}
		// if this event has an m.relationship, add on the room_id and servers to unsigned
		parent, child, relType := parentChildEventIDs(he)
		if parent == "" || child == "" || relType == "" {
			return
		}
		event, joinedToRoom := getEventIfVisible(ctx, rsAPI, parent, userID)
		if !joinedToRoom {
			return
		}
		err = he.SetUnsignedField(constRoomIDKey, event.RoomID())
		if err != nil {
			util.GetLogger(context.Background()).WithError(err).Warn("Failed to SetUnsignedField")
			return
		}

		var servers []gomatrixserverlib.ServerName
		if fsAPI != nil {
			var res fs.QueryJoinedHostServerNamesInRoomResponse
			err = fsAPI.QueryJoinedHostServerNamesInRoom(ctx, &fs.QueryJoinedHostServerNamesInRoomRequest{
				RoomID: event.RoomID(),
			}, &res)
			if err != nil {
				util.GetLogger(context.Background()).WithError(err).Warn("Failed to QueryJoinedHostServerNamesInRoom")
				return
			}
			servers = res.ServerNames
		} else {
			servers = []gomatrixserverlib.ServerName{
				base.Cfg.Global.ServerName,
			}
		}
		err = he.SetUnsignedField(constRoomServers, servers)
		if err != nil {
			util.GetLogger(context.Background()).WithError(err).Warn("Failed to SetUnsignedField")
			return
		}
	})

	base.PublicClientAPIMux.Handle("/unstable/event_relationships",
		httputil.MakeAuthAPI("eventRelationships", userAPI, eventRelationshipHandler(db, rsAPI)),
	).Methods(http.MethodPost, http.MethodOptions)
	return nil
}

type reqCtx struct {
	ctx    context.Context
	rsAPI  roomserver.RoomserverInternalAPI
	req    *EventRelationshipRequest
	userID string
}

func eventRelationshipHandler(db Database, rsAPI roomserver.RoomserverInternalAPI) func(*http.Request, *userapi.Device) util.JSONResponse {
	return func(req *http.Request, device *userapi.Device) util.JSONResponse {
		var relation EventRelationshipRequest
		if err := json.NewDecoder(req.Body).Decode(&relation); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("failed to decode HTTP request as JSON")
			return util.JSONResponse{
				Code: 400,
				JSON: jsonerror.BadJSON(fmt.Sprintf("invalid json: %s", err)),
			}
		}
		// Sanity check request and set defaults.
		relation.applyDefaults()
		var res EventRelationshipResponse
		var returnEvents []*gomatrixserverlib.HeaderedEvent
		rc := reqCtx{
			ctx:    req.Context(),
			req:    &relation,
			userID: device.UserID,
			rsAPI:  rsAPI,
		}

		// Can the user see (according to history visibility) event_id? If no, reject the request, else continue.
		event := rc.getEventIfVisible(relation.EventID, nil)
		if event == nil {
			return util.JSONResponse{
				Code: 403,
				JSON: jsonerror.Forbidden("Event does not exist or you are not authorised to see it"),
			}
		}

		// Retrieve the event. Add it to response array.
		returnEvents = append(returnEvents, event)

		if *relation.IncludeParent {
			if parentEvent := rc.includeParent(event); parentEvent != nil {
				returnEvents = append(returnEvents, parentEvent)
			}
		}

		if *relation.IncludeChildren {
			remaining := relation.Limit - len(returnEvents)
			if remaining > 0 {
				children, resErr := rc.includeChildren(db, event.EventID(), remaining, *relation.RecentFirst)
				if resErr != nil {
					return *resErr
				}
				returnEvents = append(returnEvents, children...)
			}
		}

		remaining := relation.Limit - len(returnEvents)
		var walkLimited bool
		if remaining > 0 {
			included := make(map[string]bool, len(returnEvents))
			for _, ev := range returnEvents {
				included[ev.EventID()] = true
			}
			var events []*gomatrixserverlib.HeaderedEvent
			events, walkLimited = walkThread(
				req.Context(), db, &rc, included, remaining,
			)
			returnEvents = append(returnEvents, events...)
		}
		res.Events = make([]gomatrixserverlib.ClientEvent, len(returnEvents))
		for i, ev := range returnEvents {
			res.Events[i] = gomatrixserverlib.HeaderedToClientEvent(*ev, gomatrixserverlib.FormatAll)
		}
		res.Limited = remaining == 0 || walkLimited

		return util.JSONResponse{
			Code: 200,
			JSON: res,
		}
	}
}

// If include_parent: true and there is a valid m.relationship field in the event,
// retrieve the referenced event. Apply history visibility check to that event and if it passes, add it to the response array.
func (rc *reqCtx) includeParent(event *gomatrixserverlib.HeaderedEvent) (parent *gomatrixserverlib.HeaderedEvent) {
	parentID, _, _ := parentChildEventIDs(event)
	if parentID == "" {
		return nil
	}
	return rc.getEventIfVisible(parentID, event)
}

// If include_children: true, lookup all events which have event_id as an m.relationship
// Apply history visibility checks to all these events and add the ones which pass into the response array,
// honouring the recent_first flag and the limit.
func (rc *reqCtx) includeChildren(db Database, parentID string, limit int, recentFirst bool) ([]*gomatrixserverlib.HeaderedEvent, *util.JSONResponse) {
	children, err := db.ChildrenForParent(rc.ctx, parentID, constRelType, recentFirst)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("failed to get ChildrenForParent")
		resErr := jsonerror.InternalServerError()
		return nil, &resErr
	}
	var childEvents []*gomatrixserverlib.HeaderedEvent
	for _, child := range children {
		childEvent := rc.getEventIfVisible(child.EventID, nil)
		if childEvent != nil {
			childEvents = append(childEvents, childEvent)
		}
	}
	if len(childEvents) > limit {
		return childEvents[:limit], nil
	}
	return childEvents, nil
}

// Begin to walk the thread DAG in the direction specified, either depth or breadth first according to the depth_first flag,
// honouring the limit, max_depth and max_breadth values according to the following rules
// nolint: unparam
func walkThread(
	ctx context.Context, db Database, rc *reqCtx, included map[string]bool, limit int,
) ([]*gomatrixserverlib.HeaderedEvent, bool) {
	if rc.req.Direction != "down" {
		util.GetLogger(ctx).Error("not implemented: direction=up")
		return nil, false
	}
	var result []*gomatrixserverlib.HeaderedEvent
	eventWalker := walker{
		ctx: ctx,
		req: rc.req,
		db:  db,
		fn: func(wi *walkInfo) bool {
			// If already processed event, skip.
			if included[wi.EventID] {
				return false
			}

			// If the response array is >= limit, stop.
			if len(result) >= limit {
				return true
			}

			// Process the event.
			event := rc.getEventIfVisible(wi.EventID, nil)
			if event != nil {
				result = append(result, event)
			}
			included[wi.EventID] = true
			return false
		},
	}
	limited, err := eventWalker.WalkFrom(rc.req.EventID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Errorf("Failed to WalkFrom %s", rc.req.EventID)
	}
	return result, limited
}

func (rc *reqCtx) getEventIfVisible(eventID string, child *gomatrixserverlib.HeaderedEvent) *gomatrixserverlib.HeaderedEvent {
	event, joinedToRoom := getEventIfVisible(rc.ctx, rc.rsAPI, eventID, rc.userID)
	if event == nil {
		return nil
	}
	if joinedToRoom {
		return event
	}
	if rc.req.AutoJoin {
		var joinRes roomserver.PerformJoinResponse
		rc.rsAPI.PerformJoin(rc.ctx, &roomserver.PerformJoinRequest{
			UserID:        rc.userID,
			Content:       map[string]interface{}{},
			RoomIDOrAlias: event.RoomID(),
			// TODO: Add server_names from linked room, currently this join will only work if the HS is already in the room
		}, &joinRes)
		if joinRes.Error != nil {
			util.GetLogger(rc.ctx).WithError(joinRes.Error).WithField("room_id", event.RoomID()).Error("Failed to auto-join room")
			return nil
		}
		return event
	}
	util.GetLogger(rc.ctx).Infof("user not in room and auto_join disabled")
	return nil
}

func getEventIfVisible(ctx context.Context, rsAPI roomserver.RoomserverInternalAPI, eventID, userID string) (*gomatrixserverlib.HeaderedEvent, bool) {
	var queryEventsRes roomserver.QueryEventsByIDResponse
	err := rsAPI.QueryEventsByID(ctx, &roomserver.QueryEventsByIDRequest{
		EventIDs: []string{eventID},
	}, &queryEventsRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("getEventIfVisible: failed to QueryEventsByID")
		return nil, false
	}
	if len(queryEventsRes.Events) == 0 {
		util.GetLogger(ctx).Infof("event does not exist")
		return nil, false // event does not exist
	}
	event := queryEventsRes.Events[0]

	// Allow events if the member is in the room
	// TODO: This does not honour history_visibility
	// TODO: This does not honour m.room.create content
	var queryMembershipRes roomserver.QueryMembershipForUserResponse
	err = rsAPI.QueryMembershipForUser(ctx, &roomserver.QueryMembershipForUserRequest{
		RoomID: event.RoomID(),
		UserID: userID,
	}, &queryMembershipRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("getEventIfVisible: failed to QueryMembershipForUser")
		return nil, false
	}
	return &event, queryMembershipRes.IsInRoom
}

type walkInfo struct {
	eventInfo
	SiblingNumber int
	Depth         int
}

type walker struct {
	ctx context.Context
	req *EventRelationshipRequest
	db  Database
	fn  func(wi *walkInfo) bool // callback invoked for each event walked, return true to terminate the walk
}

// WalkFrom the event ID given
func (w *walker) WalkFrom(eventID string) (limited bool, err error) {
	children, err := w.db.ChildrenForParent(w.ctx, eventID, constRelType, *w.req.RecentFirst)
	if err != nil {
		util.GetLogger(w.ctx).WithError(err).Error("WalkFrom() ChildrenForParent failed, cannot walk")
		return false, err
	}
	var next *walkInfo
	toWalk := w.addChildren(nil, children, 1)
	next, toWalk = w.nextChild(toWalk)
	for next != nil {
		stop := w.fn(next)
		if stop {
			return true, nil
		}
		// find the children's children
		children, err = w.db.ChildrenForParent(w.ctx, next.EventID, constRelType, *w.req.RecentFirst)
		if err != nil {
			util.GetLogger(w.ctx).WithError(err).Error("WalkFrom() ChildrenForParent failed, cannot walk")
			return false, err
		}
		toWalk = w.addChildren(toWalk, children, next.Depth+1)
		next, toWalk = w.nextChild(toWalk)
	}

	return false, nil
}

// addChildren adds an event's children to the to walk data structure
func (w *walker) addChildren(toWalk []walkInfo, children []eventInfo, depthOfChildren int) []walkInfo {
	// Check what number child this event is (ordered by recent_first) compared to its parent, does it exceed (greater than) max_breadth? If yes, skip.
	if len(children) > w.req.MaxBreadth {
		children = children[:w.req.MaxBreadth]
	}
	// Check how deep the event is compared to event_id, does it exceed (greater than) max_depth? If yes, skip.
	if depthOfChildren > w.req.MaxDepth {
		return toWalk
	}

	if *w.req.DepthFirst {
		// the slice is a stack so push them in reverse order so we pop them in the correct order
		// e.g [3,2,1] => [3,2] , 1 => [3] , 2 => [] , 3
		for i := len(children) - 1; i >= 0; i-- {
			toWalk = append(toWalk, walkInfo{
				eventInfo:     children[i],
				SiblingNumber: i + 1, // index from 1
				Depth:         depthOfChildren,
			})
		}
	} else {
		// the slice is a queue so push them in normal order to we dequeue them in the correct order
		// e.g [1,2,3] => 1, [2, 3] => 2 , [3] => 3, []
		for i := range children {
			toWalk = append(toWalk, walkInfo{
				eventInfo:     children[i],
				SiblingNumber: i + 1, // index from 1
				Depth:         depthOfChildren,
			})
		}
	}
	return toWalk
}

func (w *walker) nextChild(toWalk []walkInfo) (*walkInfo, []walkInfo) {
	if len(toWalk) == 0 {
		return nil, nil
	}
	var child walkInfo
	if *w.req.DepthFirst {
		// toWalk is a stack so pop the child off
		child, toWalk = toWalk[len(toWalk)-1], toWalk[:len(toWalk)-1]
		return &child, toWalk
	}
	// toWalk is a queue so shift the child off
	child, toWalk = toWalk[0], toWalk[1:]
	return &child, toWalk
}
