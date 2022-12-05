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
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/fulltext"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/userapi/api"
)

// nolint:gocyclo
func Search(req *http.Request, device *api.Device, syncDB storage.Database, fts *fulltext.Search, from *string) util.JSONResponse {
	start := time.Now()
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
	if from != nil && *from != "" {
		nextBatch, err = strconv.Atoi(*from)
		if err != nil {
			return jsonerror.InternalServerError()
		}
	}

	if searchReq.SearchCategories.RoomEvents.Filter.Limit == 0 {
		searchReq.SearchCategories.RoomEvents.Filter.Limit = 5
	}

	snapshot, err := syncDB.NewDatabaseSnapshot(req.Context())
	if err != nil {
		return jsonerror.InternalServerError()
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(snapshot, &succeeded, &err)

	// only search rooms the user is actually joined to
	joinedRooms, err := snapshot.RoomIDsWithMembership(ctx, device.UserID, "join")
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
			Code: http.StatusForbidden,
			JSON: jsonerror.Unknown("User not allowed to search in this room(s)."),
		}
	}

	orderByTime := searchReq.SearchCategories.RoomEvents.OrderBy == "recent"

	result, err := fts.Search(
		searchReq.SearchCategories.RoomEvents.SearchTerm,
		rooms,
		searchReq.SearchCategories.RoomEvents.Keys,
		searchReq.SearchCategories.RoomEvents.Filter.Limit,
		nextBatch,
		orderByTime,
	)
	if err != nil {
		logrus.WithError(err).Error("failed to search fulltext")
		return jsonerror.InternalServerError()
	}
	logrus.Debugf("Search took %s", result.Took)

	// From was specified but empty, return no results, only the count
	if from != nil && *from == "" {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: SearchResponse{
				SearchCategories: SearchCategories{
					RoomEvents: RoomEvents{
						Count:     int(result.Total),
						NextBatch: nil,
					},
				},
			},
		}
	}

	results := []Result{}

	wantEvents := make([]string, 0, len(result.Hits))
	eventScore := make(map[string]*search.DocumentMatch)

	for _, hit := range result.Hits {
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

	// Sort the events by depth, as the returned values aren't ordered
	if orderByTime {
		sort.Slice(evs, func(i, j int) bool {
			return evs[i].Depth() > evs[j].Depth()
		})
	}

	stateForRooms := make(map[string][]gomatrixserverlib.ClientEvent)
	for _, event := range evs {
		eventsBefore, eventsAfter, err := contextEvents(ctx, snapshot, event, roomFilter, searchReq)
		if err != nil {
			logrus.WithError(err).Error("failed to get context events")
			return jsonerror.InternalServerError()
		}
		startToken, endToken, err := getStartEnd(ctx, snapshot, eventsBefore, eventsAfter)
		if err != nil {
			logrus.WithError(err).Error("failed to get start/end")
			return jsonerror.InternalServerError()
		}

		profileInfos := make(map[string]ProfileInfo)
		for _, ev := range append(eventsBefore, eventsAfter...) {
			profile, ok := knownUsersProfiles[event.Sender()]
			if !ok {
				stateEvent, err := snapshot.GetStateEvent(ctx, ev.RoomID(), gomatrixserverlib.MRoomMember, ev.Sender())
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

		results = append(results, Result{
			Context: SearchContextResponse{
				Start:        startToken.String(),
				End:          endToken.String(),
				EventsAfter:  gomatrixserverlib.HeaderedToClientEvents(eventsAfter, gomatrixserverlib.FormatSync),
				EventsBefore: gomatrixserverlib.HeaderedToClientEvents(eventsBefore, gomatrixserverlib.FormatSync),
				ProfileInfo:  profileInfos,
			},
			Rank:   eventScore[event.EventID()].Score,
			Result: gomatrixserverlib.HeaderedToClientEvent(event, gomatrixserverlib.FormatAll),
		})
		roomGroup := groups[event.RoomID()]
		roomGroup.Results = append(roomGroup.Results, event.EventID())
		groups[event.RoomID()] = roomGroup
		if _, ok := stateForRooms[event.RoomID()]; searchReq.SearchCategories.RoomEvents.IncludeState && !ok {
			stateFilter := gomatrixserverlib.DefaultStateFilter()
			state, err := snapshot.CurrentState(ctx, event.RoomID(), &stateFilter, nil)
			if err != nil {
				logrus.WithError(err).Error("unable to get current state")
				return jsonerror.InternalServerError()
			}
			stateForRooms[event.RoomID()] = gomatrixserverlib.HeaderedToClientEvents(state, gomatrixserverlib.FormatSync)
		}
	}

	var nextBatchResult *string = nil
	if int(result.Total) > nextBatch+len(results) {
		nb := strconv.Itoa(len(results) + nextBatch)
		nextBatchResult = &nb
	} else if int(result.Total) == nextBatch+len(results) {
		// Sytest expects a next_batch even if we don't actually have any more results
		nb := ""
		nextBatchResult = &nb
	}

	res := SearchResponse{
		SearchCategories: SearchCategories{
			RoomEvents: RoomEvents{
				Count:      int(result.Total),
				Groups:     Groups{RoomID: groups},
				Results:    results,
				NextBatch:  nextBatchResult,
				Highlights: strings.Split(searchReq.SearchCategories.RoomEvents.SearchTerm, " "),
				State:      stateForRooms,
			},
		},
	}

	logrus.Debugf("Full search request took %v", time.Since(start))

	succeeded = true
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}

// contextEvents returns the events around a given eventID
func contextEvents(
	ctx context.Context,
	snapshot storage.DatabaseTransaction,
	event *gomatrixserverlib.HeaderedEvent,
	roomFilter *gomatrixserverlib.RoomEventFilter,
	searchReq SearchRequest,
) ([]*gomatrixserverlib.HeaderedEvent, []*gomatrixserverlib.HeaderedEvent, error) {
	id, _, err := snapshot.SelectContextEvent(ctx, event.RoomID(), event.EventID())
	if err != nil {
		logrus.WithError(err).Error("failed to query context event")
		return nil, nil, err
	}
	roomFilter.Limit = searchReq.SearchCategories.RoomEvents.EventContext.BeforeLimit
	eventsBefore, err := snapshot.SelectContextBeforeEvent(ctx, id, event.RoomID(), roomFilter)
	if err != nil {
		logrus.WithError(err).Error("failed to query before context event")
		return nil, nil, err
	}
	roomFilter.Limit = searchReq.SearchCategories.RoomEvents.EventContext.AfterLimit
	_, eventsAfter, err := snapshot.SelectContextAfterEvent(ctx, id, event.RoomID(), roomFilter)
	if err != nil {
		logrus.WithError(err).Error("failed to query after context event")
		return nil, nil, err
	}
	return eventsBefore, eventsAfter, err
}

type SearchRequest struct {
	SearchCategories struct {
		RoomEvents struct {
			EventContext struct {
				AfterLimit     int  `json:"after_limit,omitempty"`
				BeforeLimit    int  `json:"before_limit,omitempty"`
				IncludeProfile bool `json:"include_profile,omitempty"`
			} `json:"event_context"`
			Filter    gomatrixserverlib.RoomEventFilter `json:"filter"`
			Groupings struct {
				GroupBy []struct {
					Key string `json:"key"`
				} `json:"group_by"`
			} `json:"groupings"`
			IncludeState bool     `json:"include_state"`
			Keys         []string `json:"keys"`
			OrderBy      string   `json:"order_by"`
			SearchTerm   string   `json:"search_term"`
		} `json:"room_events"`
	} `json:"search_categories"`
}

type SearchResponse struct {
	SearchCategories SearchCategories `json:"search_categories"`
}
type RoomResult struct {
	NextBatch *string  `json:"next_batch,omitempty"`
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
	End          string                          `json:"end"`
	EventsAfter  []gomatrixserverlib.ClientEvent `json:"events_after"`
	EventsBefore []gomatrixserverlib.ClientEvent `json:"events_before"`
	Start        string                          `json:"start"`
	ProfileInfo  map[string]ProfileInfo          `json:"profile_info"`
}

type ProfileInfo struct {
	AvatarURL   string `json:"avatar_url"`
	DisplayName string `json:"display_name"`
}

type RoomEvents struct {
	Count      int                                        `json:"count"`
	Groups     Groups                                     `json:"groups"`
	Highlights []string                                   `json:"highlights"`
	NextBatch  *string                                    `json:"next_batch,omitempty"`
	Results    []Result                                   `json:"results"`
	State      map[string][]gomatrixserverlib.ClientEvent `json:"state,omitempty"`
}
type SearchCategories struct {
	RoomEvents RoomEvents `json:"room_events"`
}
