// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"net/http"
	"strconv"
	"strings"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/fulltext"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

func Search(req *http.Request, device *api.Device, syncDB storage.Database, fts *fulltext.Search, from string) util.JSONResponse {
	var (
		searchReq SearchRequest
		err       error
		ctx       = req.Context()
	)
	resErr := httputil.UnmarshalJSONRequest(req, &searchReq)
	if resErr != nil {
		logrus.Error("failed to unmarshal search request")
		return *resErr
	}

	nextBatch := 0
	if from != "" {
		nextBatch, err = strconv.Atoi(from)
		if err != nil {
			return jsonerror.InternalServerError()
		}
	}

	if searchReq.SearchCategories.RoomEvents.Filter.Limit == 0 {
		searchReq.SearchCategories.RoomEvents.Filter.Limit = 5
	}

	// only search rooms the user is actually joined to
	joinedRooms, err := syncDB.RoomIDsWithMembership(ctx, device.UserID, "join")
	if err != nil {
		return jsonerror.InternalServerError()
	}
	if len(joinedRooms) == 0 {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("User not joined to any rooms."),
		}
	}
	joinedRoomsMap := make(map[string]struct{}, len(joinedRooms))
	for _, roomID := range joinedRooms {
		joinedRoomsMap[roomID] = struct{}{}
	}
	rooms := []string{}
	if searchReq.SearchCategories.RoomEvents.Filter.Rooms != nil {
		for _, roomID := range *searchReq.SearchCategories.RoomEvents.Filter.Rooms {
			if _, ok := joinedRoomsMap[roomID]; ok {
				rooms = append(rooms, roomID)
			}
		}
	} else {
		rooms = joinedRooms
	}

	if len(rooms) == 0 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown("User not allowed to search in this room(s)."),
		}
	}

	logrus.Debugf("Searching FTS for rooms %v - %s", rooms, searchReq.SearchCategories.RoomEvents.SearchTerm)

	orderByTime := false
	if searchReq.SearchCategories.RoomEvents.OrderBy == "recent" {
		logrus.Debugf("Ordering by recently added")
		orderByTime = true
	}

	result, err := fts.Search(
		searchReq.SearchCategories.RoomEvents.SearchTerm,
		rooms,
		[]string{},
		searchReq.SearchCategories.RoomEvents.Filter.Limit,
		nextBatch,
		orderByTime,
	)
	if err != nil {
		logrus.WithError(err).Error("failed to search fulltext")
		return jsonerror.InternalServerError()
	}
	logrus.Debugf("Search took %s", result.Took)

	results := []Result{}

	wantEvents := make([]string, len(result.Hits))
	eventScore := make(map[string]*search.DocumentMatch)

	for _, hit := range result.Hits {
		logrus.Debugf("%+v\n", hit.Fields)
		wantEvents = append(wantEvents, hit.ID)
		eventScore[hit.ID] = hit
	}

	// Filter on m.room.message, as otherwise we also get events like m.reaction
	// which "breaks" displaying results in Element Web.
	types := []string{"m.room.message"}
	roomFilter := &gomatrixserverlib.RoomEventFilter{
		Rooms: &rooms,
		Types: &types,
	}

	evs, err := syncDB.Events(ctx, wantEvents)
	if err != nil {
		logrus.WithError(err).Error("failed to get events from database")
		return jsonerror.InternalServerError()
	}

	groups := make(map[string]RoomResult)
	knownUsersProfiles := make(map[string]ProfileInfo)
	for _, event := range evs {
		id, _, err := syncDB.SelectContextEvent(ctx, event.RoomID(), event.EventID())
		if err != nil {
			logrus.WithError(err).Error("failed to query context event")
			return jsonerror.InternalServerError()
		}
		roomFilter.Limit = searchReq.SearchCategories.RoomEvents.EventContext.BeforeLimit
		eventsBefore, err := syncDB.SelectContextBeforeEvent(ctx, id, event.RoomID(), roomFilter)
		if err != nil {
			logrus.WithError(err).Error("failed to query before context event")
			return jsonerror.InternalServerError()
		}
		roomFilter.Limit = searchReq.SearchCategories.RoomEvents.EventContext.AfterLimit
		_, eventsAfter, err := syncDB.SelectContextAfterEvent(ctx, id, event.RoomID(), roomFilter)
		if err != nil {
			logrus.WithError(err).Error("failed to query after context event")
			return jsonerror.InternalServerError()
		}

		profileInfos := make(map[string]ProfileInfo)
		for _, ev := range append(eventsBefore, eventsAfter...) {
			profile, ok := knownUsersProfiles[event.Sender()]
			if !ok {
				stateEvent, err := syncDB.GetStateEvent(ctx, ev.RoomID(), gomatrixserverlib.MRoomMember, ev.Sender())
				if err != nil {
					logrus.WithError(err).WithField("user_id", event.Sender()).Warn("failed to query userprofile")
					continue
				}
				if stateEvent == nil {
					continue
				}
				profile = ProfileInfo{
					AvatarURL:   gjson.GetBytes(stateEvent.Content(), "avatar_url").Str,
					DisplayName: gjson.GetBytes(stateEvent.Content(), "displayname").Str,
				}
				knownUsersProfiles[event.Sender()] = profile
			}
			profileInfos[ev.Sender()] = profile
		}

		r := gomatrixserverlib.HeaderedToClientEvent(event, gomatrixserverlib.FormatAll)
		results = append(results, Result{
			Context: SearchContextResponse{
				EventsAfter:  gomatrixserverlib.HeaderedToClientEvents(eventsAfter, gomatrixserverlib.FormatSync),
				EventsBefore: gomatrixserverlib.HeaderedToClientEvents(eventsBefore, gomatrixserverlib.FormatSync),
				ProfileInfo:  profileInfos,
			},
			Rank:   eventScore[event.EventID()].Score,
			Result: r,
		})
		roomGroup := groups[event.RoomID()]
		roomGroup.Results = append(roomGroup.Results, event.EventID())
		groups[event.RoomID()] = roomGroup
	}

	nb := ""
	if int(result.Total) > nextBatch+len(results) {
		nb = strconv.Itoa(len(results) + nextBatch)
	}

	res := SearchResponse{
		SearchCategories: SearchCategories{
			RoomEvents: RoomEvents{
				Count:      int(result.Total),
				Groups:     Groups{RoomID: groups},
				Results:    results,
				NextBatch:  nb,
				Highlights: strings.Split(searchReq.SearchCategories.RoomEvents.SearchTerm, " "),
			},
		},
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}

type SearchRequest struct {
	SearchCategories struct {
		RoomEvents struct {
			EventContext struct {
				AfterLimit     int  `json:"after_limit,omitempty"`
				BeforeLimit    int  `json:"before_limit,omitempty"`
				IncludeProfile bool `json:"include_profile,omitempty"`
			} `json:"event_context"`
			Filter    gomatrixserverlib.StateFilter `json:"filter"`
			Groupings struct {
				GroupBy []struct {
					Key string `json:"key"`
				} `json:"group_by"`
			} `json:"groupings"`
			Keys       []string `json:"keys"`
			OrderBy    string   `json:"order_by"`
			SearchTerm string   `json:"search_term"`
		} `json:"room_events"`
	} `json:"search_categories"`
}

type SearchResponse struct {
	SearchCategories SearchCategories `json:"search_categories"`
}
type RoomResult struct {
	NextBatch string   `json:"next_batch"`
	Order     int      `json:"order"`
	Results   []string `json:"results"`
}

type Groups struct {
	RoomID map[string]RoomResult `json:"room_id"`
}

type Result struct {
	Context SearchContextResponse         `json:"context"`
	Rank    float64                       `json:"rank"`
	Result  gomatrixserverlib.ClientEvent `json:"result"`
}

type SearchContextResponse struct {
	End          string                          `json:"end"` // TODO
	EventsAfter  []gomatrixserverlib.ClientEvent `json:"events_after"`
	EventsBefore []gomatrixserverlib.ClientEvent `json:"events_before"`
	Start        string                          `json:"start"`        // TODO
	ProfileInfo  map[string]ProfileInfo          `json:"profile_info"` // TODO
}

type ProfileInfo struct {
	AvatarURL   string `json:"avatar_url"`   // TODO
	DisplayName string `json:"display_name"` // TODO
}

type RoomEvents struct {
	Count      int      `json:"count"`
	Groups     Groups   `json:"groups"`
	Highlights []string `json:"highlights"`
	NextBatch  string   `json:"next_batch"`
	Results    []Result `json:"results"`
	State      struct{} `json:"state"` // TODO
}
type SearchCategories struct {
	RoomEvents RoomEvents `json:"room_events"`
}
