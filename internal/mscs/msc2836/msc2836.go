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
	"sort"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/setup"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
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

type eventRelationshipResponse struct {
	Events    []gomatrixserverlib.ClientEvent `json:"events"`
	NextBatch string                          `json:"next_batch"`
	Limited   bool                            `json:"limited"`
}

// Enable this MSC
func Enable(base *setup.BaseDendrite, userAPI userapi.UserInternalAPI) error {
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
		httputil.MakeAuthAPI("eventRelationships", userAPI, func(req *http.Request, device *userapi.Device) util.JSONResponse {
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
			var res eventRelationshipResponse
			var returnEvents []*gomatrixserverlib.HeaderedEvent

			// Can the user see (according to history visibility) event_id? If no, reject the request, else continue.
			event := getEventIfVisible(req.Context(), relation.EventID, device.UserID)
			if event == nil {
				return util.JSONResponse{
					Code: 403,
					JSON: jsonerror.Forbidden("Event does not exist or you are not authorised to see it"),
				}
			}

			// Retrieve the event. Add it to response array.
			returnEvents = append(returnEvents, event)

			if *relation.IncludeParent {
				if parentEvent := includeParent(req.Context(), event, device.UserID); parentEvent != nil {
					returnEvents = append(returnEvents, parentEvent)
				}
			}

			if *relation.IncludeChildren {
				remaining := relation.Limit - len(returnEvents)
				if remaining > 0 {
					children, resErr := includeChildren(req.Context(), db, event.EventID(), remaining, *relation.RecentFirst, device.UserID)
					if resErr != nil {
						return *resErr
					}
					returnEvents = append(returnEvents, children...)
				}
			}

			remaining := relation.Limit - len(returnEvents)
			var walkLimited bool
			if remaining > 0 {
				// Begin to walk the thread DAG in the direction specified, either depth or breadth first according to the depth_first flag,
				// honouring the limit, max_depth and max_breadth values according to the following rules
				var events []*gomatrixserverlib.HeaderedEvent
				events, walkLimited = walkThread(req.Context(), db, remaining)
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
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	return nil
}

// If include_parent: true and there is a valid m.relationship field in the event,
// retrieve the referenced event. Apply history visibility check to that event and if it passes, add it to the response array.
func includeParent(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, userID string) (parent *gomatrixserverlib.HeaderedEvent) {
	parentID, _ := parentChildEventIDs(event)
	if parentID == "" {
		return nil
	}
	return getEventIfVisible(ctx, parentID, userID)
}

// If include_children: true, lookup all events which have event_id as an m.relationship
// Apply history visibility checks to all these events and add the ones which pass into the response array,
// honouring the recent_first flag and the limit.
func includeChildren(ctx context.Context, db Database, parentID string, limit int, recentFirst bool, userID string) ([]*gomatrixserverlib.HeaderedEvent, *util.JSONResponse) {
	children, err := db.ChildrenForParent(ctx, parentID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("failed to get ChildrenForParent")
		resErr := jsonerror.InternalServerError()
		return nil, &resErr
	}
	var childEvents []*gomatrixserverlib.HeaderedEvent
	for _, child := range children {
		childEvent := getEventIfVisible(ctx, child, userID)
		if childEvent != nil {
			childEvents = append(childEvents, childEvent)
		}
	}
	// sort childEvents by origin_server_ts in ASC or DESC depending on recent_first
	sort.SliceStable(childEvents, func(i, j int) bool {
		if recentFirst {
			return childEvents[i].OriginServerTS().Time().After(childEvents[j].OriginServerTS().Time())
		}
		return childEvents[i].OriginServerTS().Time().Before(childEvents[j].OriginServerTS().Time())
	})
	if len(childEvents) > limit {
		return childEvents[:limit], nil
	}
	return childEvents, nil
}

func walkThread(ctx context.Context, db Database, limit int) ([]*gomatrixserverlib.HeaderedEvent, bool) {
	return nil, false
}

func getEventIfVisible(ctx context.Context, eventID, userID string) *gomatrixserverlib.HeaderedEvent {
	return nil
}
