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
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/setup"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const constRelType = "m.reference"

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
func Enable(base *setup.BaseDendrite, rsAPI roomserver.RoomserverInternalAPI, userAPI userapi.UserInternalAPI) error {
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

	base.PublicClientAPIMux.Handle("/unstable/event_relationships",
		httputil.MakeAuthAPI("eventRelationships", userAPI, eventRelationshipHandler(db, rsAPI)),
	).Methods(http.MethodPost, http.MethodOptions)
	return nil
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

		// Can the user see (according to history visibility) event_id? If no, reject the request, else continue.
		event := getEventIfVisible(req.Context(), rsAPI, relation.EventID, device.UserID)
		if event == nil {
			return util.JSONResponse{
				Code: 403,
				JSON: jsonerror.Forbidden("Event does not exist or you are not authorised to see it"),
			}
		}

		// Retrieve the event. Add it to response array.
		returnEvents = append(returnEvents, event)

		if *relation.IncludeParent {
			if parentEvent := includeParent(req.Context(), rsAPI, event, device.UserID); parentEvent != nil {
				returnEvents = append(returnEvents, parentEvent)
			}
		}

		if *relation.IncludeChildren {
			remaining := relation.Limit - len(returnEvents)
			if remaining > 0 {
				children, resErr := includeChildren(req.Context(), rsAPI, db, event.EventID(), remaining, *relation.RecentFirst, device.UserID)
				if resErr != nil {
					return *resErr
				}
				returnEvents = append(returnEvents, children...)
			}
		}

		remaining := relation.Limit - len(returnEvents)
		var walkLimited bool
		if remaining > 0 {
			depths := make(map[string]int, len(returnEvents))
			for _, ev := range returnEvents {
				depths[ev.EventID()] = 1
			}
			var events []*gomatrixserverlib.HeaderedEvent
			events, walkLimited = walkThread(
				req.Context(), db, rsAPI, device.UserID, &relation, depths, remaining,
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
func includeParent(ctx context.Context, rsAPI roomserver.RoomserverInternalAPI, event *gomatrixserverlib.HeaderedEvent, userID string) (parent *gomatrixserverlib.HeaderedEvent) {
	parentID, _, _ := parentChildEventIDs(event)
	if parentID == "" {
		return nil
	}
	return getEventIfVisible(ctx, rsAPI, parentID, userID)
}

// If include_children: true, lookup all events which have event_id as an m.relationship
// Apply history visibility checks to all these events and add the ones which pass into the response array,
// honouring the recent_first flag and the limit.
func includeChildren(ctx context.Context, rsAPI roomserver.RoomserverInternalAPI, db Database, parentID string, limit int, recentFirst bool, userID string) ([]*gomatrixserverlib.HeaderedEvent, *util.JSONResponse) {
	children, err := db.ChildrenForParent(ctx, parentID, constRelType, recentFirst)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("failed to get ChildrenForParent")
		resErr := jsonerror.InternalServerError()
		return nil, &resErr
	}
	var childEvents []*gomatrixserverlib.HeaderedEvent
	for _, child := range children {
		childEvent := getEventIfVisible(ctx, rsAPI, child.EventID, userID)
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
	ctx context.Context, db Database, rsAPI roomserver.RoomserverInternalAPI, userID string, req *EventRelationshipRequest, included map[string]int, limit int,
) ([]*gomatrixserverlib.HeaderedEvent, bool) {
	if req.Direction != "down" {
		util.GetLogger(ctx).Error("not implemented: direction=up")
		return nil, false
	}
	var result []*gomatrixserverlib.HeaderedEvent
	eventWalker := walker{
		ctx: ctx,
		req: req,
		db:  db,
	}
	parent, current := eventWalker.Next()
	for current.EventID != "" {
		// If the response array is >= limit, stop.
		if len(result) >= limit {
			return result, true
		}
		// If already processed event, skip.
		if included[current.EventID] > 0 {
			continue
		}

		// Check how deep the event is compared to event_id, does it exceed (greater than) max_depth? If yes, skip.
		parentDepth := included[parent]
		if parentDepth == 0 {
			util.GetLogger(ctx).Errorf("parent has unknown depth; this should be impossible, parent=%s curr=%v map=%v", parent, current, included)
			// set these at the max to stop walking this part of the DAG
			included[parent] = req.MaxDepth
			included[current.EventID] = req.MaxDepth
			continue
		}
		depth := parentDepth + 1
		if depth > req.MaxDepth {
			continue
		}

		// Check what number child this event is (ordered by recent_first) compared to its parent, does it exceed (greater than) max_breadth? If yes, skip.
		if current.SiblingNumber > req.MaxBreadth {
			continue
		}

		// Process the event.
		event := getEventIfVisible(ctx, rsAPI, current.EventID, userID)
		if event != nil {
			result = append(result, event)
		}
		included[current.EventID] = depth
	}
	return result, false
}

func getEventIfVisible(ctx context.Context, rsAPI roomserver.RoomserverInternalAPI, eventID, userID string) *gomatrixserverlib.HeaderedEvent {
	var queryEventsRes roomserver.QueryEventsByIDResponse
	err := rsAPI.QueryEventsByID(ctx, &roomserver.QueryEventsByIDRequest{
		EventIDs: []string{eventID},
	}, &queryEventsRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("getEventIfVisible: failed to QueryEventsByID")
		return nil
	}
	if len(queryEventsRes.Events) == 0 {
		util.GetLogger(ctx).Infof("event does not exist")
		return nil // event does not exist
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
		return nil
	}
	if !queryMembershipRes.IsInRoom {
		util.GetLogger(ctx).Infof("user not in room")
		return nil
	}
	return &event
}

type walkInfo struct {
	eventInfo
	SiblingNumber int
}

type walker struct {
	ctx     context.Context
	req     *EventRelationshipRequest
	db      Database
	current string
	//toProcess []walkInfo
}

// Next returns the next event to process.
func (w *walker) Next() (parent string, current walkInfo) {
	//var events []string

	_, err := w.db.ChildrenForParent(w.ctx, w.current, constRelType, *w.req.RecentFirst)
	if err != nil {
		util.GetLogger(w.ctx).WithError(err).Error("Next() failed, cannot walk")
		return
	}

	return
}
