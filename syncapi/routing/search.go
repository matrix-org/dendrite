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

	results := []Results{}

	wantEvents := make([]string, len(result.Hits))
	eventScore := make(map[string]*search.DocumentMatch)
	for _, hit := range result.Hits {
		logrus.Debugf("%+v\n", hit.Fields)
		wantEvents = append(wantEvents, hit.ID)
		eventScore[hit.ID] = hit
	}

	roomFilter := &gomatrixserverlib.RoomEventFilter{
		Rooms: &rooms,
	}

	evs, err := syncDB.Events(ctx, wantEvents)
	if err != nil {
		logrus.WithError(err).Error("failed to get events from database")
		return jsonerror.InternalServerError()
	}

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

		results = append(results, Results{
			Context: ContextRespsonse{
				EventsAfter:  gomatrixserverlib.HeaderedToClientEvents(eventsAfter, gomatrixserverlib.FormatSync),
				EventsBefore: gomatrixserverlib.HeaderedToClientEvents(eventsBefore, gomatrixserverlib.FormatSync),
			},
			Rank: eventScore[event.EventID()].Score,
			Result: Result{
				Content: Content{
					Body:          gjson.GetBytes(event.Content(), "body").Str,
					Format:        gjson.GetBytes(event.Content(), "format").Str,
					FormattedBody: gjson.GetBytes(event.Content(), "formatted_body").Str,
					Msgtype:       gjson.GetBytes(event.Content(), "msgtype").Str,
				},
				EventID:        event.EventID(),
				OriginServerTs: event.OriginServerTS(),
				RoomID:         event.RoomID(),
				Sender:         event.Sender(),
				Type:           event.Type(),
				Unsigned:       event.Unsigned(),
			},
		})
	}

	nb := ""
	if int(result.Total) > nextBatch+len(results) {
		nb = strconv.Itoa(len(results) + nextBatch)
	}

	res := SearchResponse{
		SearchCategories: SearchCategories{
			RoomEvents: RoomEvents{
				Count:     int(result.Total),
				Groups:    Groups{},
				Results:   results,
				NextBatch: nb,
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
type RoomID struct {
	Room RoomResult
}
type Groups struct {
	RoomID RoomID `json:"room_id"`
}
type Content struct {
	Body          string `json:"body"`
	Format        string `json:"format"`
	FormattedBody string `json:"formatted_body"`
	Msgtype       string `json:"msgtype"`
}

type Result struct {
	Content        Content                     `json:"content"`
	EventID        string                      `json:"event_id"`
	OriginServerTs gomatrixserverlib.Timestamp `json:"origin_server_ts"`
	RoomID         string                      `json:"room_id"`
	Sender         string                      `json:"sender"`
	Type           string                      `json:"type"`
	Unsigned       []byte                      `json:"unsigned,omitempty"`
}
type Results struct {
	Context ContextRespsonse `json:"context"`
	Rank    float64          `json:"rank"`
	Result  Result           `json:"result"`
}

type RoomEvents struct {
	Count      int       `json:"count"`
	Groups     Groups    `json:"groups"`
	Highlights []string  `json:"highlights"`
	NextBatch  string    `json:"next_batch"`
	Results    []Results `json:"results"`
}
type SearchCategories struct {
	RoomEvents RoomEvents `json:"room_events"`
}
