// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/internal/fulltext"
	"github.com/element-hq/dendrite/internal/sqlutil"
	roomserverAPI "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/storage"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/userapi/api"
)

// nolint:gocyclo
func Search(req *http.Request, device *api.Device, syncDB storage.Database, fts fulltext.Indexer, from *string, rsAPI roomserverAPI.SyncRoomserverAPI) util.JSONResponse {
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
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
	}

	if searchReq.SearchCategories.RoomEvents.Filter.Limit == 0 {
		searchReq.SearchCategories.RoomEvents.Filter.Limit = 5
	}

	snapshot, err := syncDB.NewDatabaseSnapshot(req.Context())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(snapshot, &succeeded, &err)

	// only search rooms the user is actually joined to
	joinedRooms, err := snapshot.RoomIDsWithMembership(ctx, device.UserID, "join")
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if len(joinedRooms) == 0 {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("User not joined to any rooms."),
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
			JSON: spec.Unknown("User not allowed to search in this room(s)."),
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
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	logrus.Debugf("Search took %s", result.Took)

	// From was specified but empty, return no results, only the count
	if from != nil && *from == "" {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: SearchResponse{
				SearchCategories: SearchCategoriesResponse{
					RoomEvents: RoomEventsResponse{
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
	roomFilter := &synctypes.RoomEventFilter{
		Rooms: &rooms,
		Types: &types,
	}

	evs, err := syncDB.Events(ctx, wantEvents)
	if err != nil {
		logrus.WithError(err).Error("failed to get events from database")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	groups := make(map[string]RoomResult)
	knownUsersProfiles := make(map[string]ProfileInfoResponse)

	// Sort the events by depth, as the returned values aren't ordered
	if orderByTime {
		sort.Slice(evs, func(i, j int) bool {
			return evs[i].Depth() > evs[j].Depth()
		})
	}

	stateForRooms := make(map[string][]synctypes.ClientEvent)
	for _, event := range evs {
		eventsBefore, eventsAfter, err := contextEvents(ctx, snapshot, event, roomFilter, searchReq)
		if err != nil {
			logrus.WithError(err).Error("failed to get context events")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		startToken, endToken, err := getStartEnd(ctx, snapshot, eventsBefore, eventsAfter)
		if err != nil {
			logrus.WithError(err).Error("failed to get start/end")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		profileInfos := make(map[string]ProfileInfoResponse)
		for _, ev := range append(eventsBefore, eventsAfter...) {
			userID, queryErr := rsAPI.QueryUserIDForSender(req.Context(), ev.RoomID(), ev.SenderID())
			if queryErr != nil {
				logrus.WithError(queryErr).WithField("sender_id", ev.SenderID()).Warn("failed to query userprofile")
				continue
			}

			profile, ok := knownUsersProfiles[userID.String()]
			if !ok {
				stateEvent, stateErr := snapshot.GetStateEvent(ctx, ev.RoomID().String(), spec.MRoomMember, string(ev.SenderID()))
				if stateErr != nil {
					logrus.WithError(stateErr).WithField("sender_id", event.SenderID()).Warn("failed to query userprofile")
					continue
				}
				if stateEvent == nil {
					continue
				}
				profile = ProfileInfoResponse{
					AvatarURL:   gjson.GetBytes(stateEvent.Content(), "avatar_url").Str,
					DisplayName: gjson.GetBytes(stateEvent.Content(), "displayname").Str,
				}
				knownUsersProfiles[userID.String()] = profile
			}
			profileInfos[userID.String()] = profile
		}

		clientEvent, err := synctypes.ToClientEvent(event, synctypes.FormatAll, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		})
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).WithField("senderID", event.SenderID()).Error("Failed converting to ClientEvent")
			continue
		}

		results = append(results, Result{
			Context: SearchContextResponse{
				Start: startToken.String(),
				End:   endToken.String(),
				EventsAfter: synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(eventsAfter), synctypes.FormatSync, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
					return rsAPI.QueryUserIDForSender(req.Context(), roomID, senderID)
				}),
				EventsBefore: synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(eventsBefore), synctypes.FormatSync, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
					return rsAPI.QueryUserIDForSender(req.Context(), roomID, senderID)
				}),
				ProfileInfo: profileInfos,
			},
			Rank:   eventScore[event.EventID()].Score,
			Result: *clientEvent,
		})
		roomGroup := groups[event.RoomID().String()]
		roomGroup.Results = append(roomGroup.Results, event.EventID())
		groups[event.RoomID().String()] = roomGroup
		if _, ok := stateForRooms[event.RoomID().String()]; searchReq.SearchCategories.RoomEvents.IncludeState && !ok {
			stateFilter := synctypes.DefaultStateFilter()
			state, err := snapshot.CurrentState(ctx, event.RoomID().String(), &stateFilter, nil)
			if err != nil {
				logrus.WithError(err).Error("unable to get current state")
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}
			stateForRooms[event.RoomID().String()] = synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(state), synctypes.FormatSync, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
				return rsAPI.QueryUserIDForSender(req.Context(), roomID, senderID)
			})
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
		SearchCategories: SearchCategoriesResponse{
			RoomEvents: RoomEventsResponse{
				Count:      int(result.Total),
				Groups:     Groups{RoomID: groups},
				Results:    results,
				NextBatch:  nextBatchResult,
				Highlights: fts.GetHighlights(result),
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
	event *types.HeaderedEvent,
	roomFilter *synctypes.RoomEventFilter,
	searchReq SearchRequest,
) ([]*types.HeaderedEvent, []*types.HeaderedEvent, error) {
	id, _, err := snapshot.SelectContextEvent(ctx, event.RoomID().String(), event.EventID())
	if err != nil {
		logrus.WithError(err).Error("failed to query context event")
		return nil, nil, err
	}
	roomFilter.Limit = searchReq.SearchCategories.RoomEvents.EventContext.BeforeLimit
	eventsBefore, err := snapshot.SelectContextBeforeEvent(ctx, id, event.RoomID().String(), roomFilter)
	if err != nil {
		logrus.WithError(err).Error("failed to query before context event")
		return nil, nil, err
	}
	roomFilter.Limit = searchReq.SearchCategories.RoomEvents.EventContext.AfterLimit
	_, eventsAfter, err := snapshot.SelectContextAfterEvent(ctx, id, event.RoomID().String(), roomFilter)
	if err != nil {
		logrus.WithError(err).Error("failed to query after context event")
		return nil, nil, err
	}
	return eventsBefore, eventsAfter, err
}

type EventContext struct {
	AfterLimit     int  `json:"after_limit,omitempty"`
	BeforeLimit    int  `json:"before_limit,omitempty"`
	IncludeProfile bool `json:"include_profile,omitempty"`
}

type GroupBy struct {
	Key string `json:"key"`
}

type Groupings struct {
	GroupBy []GroupBy `json:"group_by"`
}

type RoomEvents struct {
	EventContext EventContext              `json:"event_context"`
	Filter       synctypes.RoomEventFilter `json:"filter"`
	Groupings    Groupings                 `json:"groupings"`
	IncludeState bool                      `json:"include_state"`
	Keys         []string                  `json:"keys"`
	OrderBy      string                    `json:"order_by"`
	SearchTerm   string                    `json:"search_term"`
}

type SearchCategories struct {
	RoomEvents RoomEvents `json:"room_events"`
}

type SearchRequest struct {
	SearchCategories SearchCategories `json:"search_categories"`
}

type SearchResponse struct {
	SearchCategories SearchCategoriesResponse `json:"search_categories"`
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
	Context SearchContextResponse `json:"context"`
	Rank    float64               `json:"rank"`
	Result  synctypes.ClientEvent `json:"result"`
}

type SearchContextResponse struct {
	End          string                         `json:"end"`
	EventsAfter  []synctypes.ClientEvent        `json:"events_after"`
	EventsBefore []synctypes.ClientEvent        `json:"events_before"`
	Start        string                         `json:"start"`
	ProfileInfo  map[string]ProfileInfoResponse `json:"profile_info"`
}

type ProfileInfoResponse struct {
	AvatarURL   string `json:"avatar_url"`
	DisplayName string `json:"display_name"`
}

type RoomEventsResponse struct {
	Count      int                                `json:"count"`
	Groups     Groups                             `json:"groups"`
	Highlights []string                           `json:"highlights"`
	NextBatch  *string                            `json:"next_batch,omitempty"`
	Results    []Result                           `json:"results"`
	State      map[string][]synctypes.ClientEvent `json:"state,omitempty"`
}
type SearchCategoriesResponse struct {
	RoomEvents RoomEventsResponse `json:"room_events"`
}
