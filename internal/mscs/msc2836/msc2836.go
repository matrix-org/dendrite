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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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
	constRelType = "m.reference"
)

type EventRelationshipRequest struct {
	EventID         string `json:"event_id"`
	MaxDepth        int    `json:"max_depth"`
	MaxBreadth      int    `json:"max_breadth"`
	Limit           int    `json:"limit"`
	DepthFirst      bool   `json:"depth_first"`
	RecentFirst     bool   `json:"recent_first"`
	IncludeParent   bool   `json:"include_parent"`
	IncludeChildren bool   `json:"include_children"`
	Direction       string `json:"direction"`
	Batch           string `json:"batch"`
	AutoJoin        bool   `json:"auto_join"`
}

func NewEventRelationshipRequest(body io.Reader) (*EventRelationshipRequest, error) {
	var relation EventRelationshipRequest
	relation.Defaults()
	if err := json.NewDecoder(body).Decode(&relation); err != nil {
		return nil, err
	}
	return &relation, nil
}

func (r *EventRelationshipRequest) Defaults() {
	r.Limit = 100
	r.MaxBreadth = 10
	r.MaxDepth = 3
	r.DepthFirst = false
	r.RecentFirst = true
	r.IncludeParent = false
	r.IncludeChildren = false
	r.Direction = "down"
}

type EventRelationshipResponse struct {
	Events    []gomatrixserverlib.ClientEvent `json:"events"`
	NextBatch string                          `json:"next_batch"`
	Limited   bool                            `json:"limited"`
}

func toClientResponse(res *gomatrixserverlib.MSC2836EventRelationshipsResponse) *EventRelationshipResponse {
	out := &EventRelationshipResponse{
		Events:    gomatrixserverlib.ToClientEvents(res.Events, gomatrixserverlib.FormatAll),
		Limited:   res.Limited,
		NextBatch: res.NextBatch,
	}
	return out
}

// Enable this MSC
// nolint:gocyclo
func Enable(
	base *setup.BaseDendrite, rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationSenderInternalAPI,
	userAPI userapi.UserInternalAPI, keyRing gomatrixserverlib.JSONVerifier,
) error {
	db, err := NewDatabase(&base.Cfg.MSCs.Database)
	if err != nil {
		return fmt.Errorf("Cannot enable MSC2836: %w", err)
	}
	hooks.Enable()
	hooks.Attach(hooks.KindNewEventPersisted, func(headeredEvent interface{}) {
		he := headeredEvent.(*gomatrixserverlib.HeaderedEvent)
		hookErr := db.StoreRelation(context.Background(), he)
		if hookErr != nil {
			util.GetLogger(context.Background()).WithError(hookErr).Error(
				"failed to StoreRelation",
			)
		}
	})

	base.PublicClientAPIMux.Handle("/unstable/event_relationships",
		httputil.MakeAuthAPI("eventRelationships", userAPI, eventRelationshipHandler(db, rsAPI, fsAPI)),
	).Methods(http.MethodPost, http.MethodOptions)

	base.PublicFederationAPIMux.Handle("/unstable/event_relationships", httputil.MakeExternalAPI(
		"msc2836_event_relationships", func(req *http.Request) util.JSONResponse {
			fedReq, errResp := gomatrixserverlib.VerifyHTTPRequest(
				req, time.Now(), base.Cfg.Global.ServerName, keyRing,
			)
			if fedReq == nil {
				return errResp
			}
			return federatedEventRelationship(req.Context(), fedReq, db, rsAPI, fsAPI)
		},
	)).Methods(http.MethodPost, http.MethodOptions)
	return nil
}

type reqCtx struct {
	ctx               context.Context
	rsAPI             roomserver.RoomserverInternalAPI
	db                Database
	req               *EventRelationshipRequest
	userID            string
	authorisedRoomIDs map[string]gomatrixserverlib.RoomVersion // events from these rooms can be returned

	// federated request args
	isFederatedRequest bool
	serverName         gomatrixserverlib.ServerName
	fsAPI              fs.FederationSenderInternalAPI
}

func eventRelationshipHandler(db Database, rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationSenderInternalAPI) func(*http.Request, *userapi.Device) util.JSONResponse {
	return func(req *http.Request, device *userapi.Device) util.JSONResponse {
		relation, err := NewEventRelationshipRequest(req.Body)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("failed to decode HTTP request as JSON")
			return util.JSONResponse{
				Code: 400,
				JSON: jsonerror.BadJSON(fmt.Sprintf("invalid json: %s", err)),
			}
		}
		rc := reqCtx{
			ctx:                req.Context(),
			req:                relation,
			userID:             device.UserID,
			rsAPI:              rsAPI,
			fsAPI:              fsAPI,
			isFederatedRequest: false,
			db:                 db,
			authorisedRoomIDs:  make(map[string]gomatrixserverlib.RoomVersion),
		}
		res, resErr := rc.process()
		if resErr != nil {
			return *resErr
		}

		return util.JSONResponse{
			Code: 200,
			JSON: toClientResponse(res),
		}
	}
}

func federatedEventRelationship(
	ctx context.Context, fedReq *gomatrixserverlib.FederationRequest, db Database, rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationSenderInternalAPI,
) util.JSONResponse {
	relation, err := NewEventRelationshipRequest(bytes.NewBuffer(fedReq.Content()))
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("failed to decode HTTP request as JSON")
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("invalid json: %s", err)),
		}
	}
	rc := reqCtx{
		ctx:               ctx,
		req:               relation,
		rsAPI:             rsAPI,
		db:                db,
		authorisedRoomIDs: make(map[string]gomatrixserverlib.RoomVersion),
		// federation args
		isFederatedRequest: true,
		fsAPI:              fsAPI,
		serverName:         fedReq.Origin(),
	}
	res, resErr := rc.process()
	if resErr != nil {
		return *resErr
	}
	// add auth chain information
	requiredAuthEventsSet := make(map[string]bool)
	var requiredAuthEvents []string
	for _, ev := range res.Events {
		for _, a := range ev.AuthEventIDs() {
			if requiredAuthEventsSet[a] {
				continue
			}
			requiredAuthEvents = append(requiredAuthEvents, a)
			requiredAuthEventsSet[a] = true
		}
	}
	var queryRes roomserver.QueryAuthChainResponse
	err = rsAPI.QueryAuthChain(ctx, &roomserver.QueryAuthChainRequest{
		EventIDs: requiredAuthEvents,
	}, &queryRes)
	if err != nil {
		// they may already have the auth events so don't fail this request
		util.GetLogger(ctx).WithError(err).Error("Failed to QueryAuthChain")
	}
	res.AuthChain = make([]*gomatrixserverlib.Event, len(queryRes.AuthChain))
	for i := range queryRes.AuthChain {
		res.AuthChain[i] = queryRes.AuthChain[i].Unwrap()
	}

	return util.JSONResponse{
		Code: 200,
		JSON: res,
	}
}

func (rc *reqCtx) process() (*gomatrixserverlib.MSC2836EventRelationshipsResponse, *util.JSONResponse) {
	var res gomatrixserverlib.MSC2836EventRelationshipsResponse
	var returnEvents []*gomatrixserverlib.HeaderedEvent
	// Can the user see (according to history visibility) event_id? If no, reject the request, else continue.
	event := rc.getLocalEvent(rc.req.EventID)
	if event == nil || !rc.authorisedToSeeEvent(event) {
		return nil, &util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("Event does not exist or you are not authorised to see it"),
		}
	}

	// Retrieve the event. Add it to response array.
	returnEvents = append(returnEvents, event)

	if rc.req.IncludeParent {
		if parentEvent := rc.includeParent(event); parentEvent != nil {
			returnEvents = append(returnEvents, parentEvent)
		}
	}

	if rc.req.IncludeChildren {
		remaining := rc.req.Limit - len(returnEvents)
		if remaining > 0 {
			children, resErr := rc.includeChildren(rc.db, event.EventID(), remaining, rc.req.RecentFirst)
			if resErr != nil {
				return nil, resErr
			}
			returnEvents = append(returnEvents, children...)
		}
	}

	remaining := rc.req.Limit - len(returnEvents)
	var walkLimited bool
	if remaining > 0 {
		included := make(map[string]bool, len(returnEvents))
		for _, ev := range returnEvents {
			included[ev.EventID()] = true
		}
		var events []*gomatrixserverlib.HeaderedEvent
		events, walkLimited = walkThread(
			rc.ctx, rc.db, rc, included, remaining,
		)
		returnEvents = append(returnEvents, events...)
	}
	res.Events = make([]*gomatrixserverlib.Event, len(returnEvents))
	for i, ev := range returnEvents {
		res.Events[i] = ev.Unwrap()
	}
	res.Limited = remaining == 0 || walkLimited
	return &res, nil
}

// If include_parent: true and there is a valid m.relationship field in the event,
// retrieve the referenced event. Apply history visibility check to that event and if it passes, add it to the response array.
func (rc *reqCtx) includeParent(childEvent *gomatrixserverlib.HeaderedEvent) (parent *gomatrixserverlib.HeaderedEvent) {
	parentID, _, _ := parentChildEventIDs(childEvent)
	if parentID == "" {
		return nil
	}
	return rc.lookForEvent(parentID, false)
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
		childEvent := rc.lookForEvent(child.EventID, false)
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
			// if event is not found, use remoteEventRelationships to explore that part of the thread remotely.
			// This will probably be easiest if the event relationships response is directly pumped into the database
			// so the next walk will do the right thing. This requires those events to be authed and likely injected as
			// outliers into the roomserver DB, which will de-dupe appropriately.
			event := rc.lookForEvent(wi.EventID, true)
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

// MSC2836EventRelationships performs an /event_relationships request to a remote server, injecting the resulting events
// into the roomserver as KindOutlier, with auth chains.
func (rc *reqCtx) MSC2836EventRelationships(eventID string, srv gomatrixserverlib.ServerName, ver gomatrixserverlib.RoomVersion) (*gomatrixserverlib.MSC2836EventRelationshipsResponse, error) {
	res, err := rc.fsAPI.MSC2836EventRelationships(rc.ctx, srv, gomatrixserverlib.MSC2836EventRelationshipsRequest{
		EventID:     eventID,
		DepthFirst:  rc.req.DepthFirst,
		Direction:   rc.req.Direction,
		Limit:       rc.req.Limit,
		MaxBreadth:  rc.req.MaxBreadth,
		MaxDepth:    rc.req.MaxDepth,
		RecentFirst: rc.req.RecentFirst,
	}, ver)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("Failed to call MSC2836EventRelationships")
		return nil, err
	}
	return &res, nil

}

// authorisedToSeeEvent authenticates that the user or server is allowed to see this event. Returns true if allowed to
// see this request.
func (rc *reqCtx) authorisedToSeeEvent(event *gomatrixserverlib.HeaderedEvent) bool {
	authorised, ok := rc.authorisedRoomIDs[event.RoomID()]
	if ok {
		return len(authorised) > 0
	}
	if rc.isFederatedRequest {
		// make sure the server is in this room
		var res fs.QueryJoinedHostServerNamesInRoomResponse
		err := rc.fsAPI.QueryJoinedHostServerNamesInRoom(rc.ctx, &fs.QueryJoinedHostServerNamesInRoomRequest{
			RoomID: event.RoomID(),
		}, &res)
		if err != nil {
			util.GetLogger(rc.ctx).WithError(err).Error("authenticateEvent: failed to QueryJoinedHostServerNamesInRoom")
			return false
		}
		for _, srv := range res.ServerNames {
			if srv == rc.serverName {
				rc.authorisedRoomIDs[event.RoomID()] = event.Version()
				return true
			}
		}
		return false
	}
	// make sure the user is in this room
	joinedToRoom, err := rc.allowedToSeeEvent(event.RoomID(), rc.userID)
	if err != nil || !joinedToRoom {
		return false
	}
	rc.authorisedRoomIDs[event.RoomID()] = event.Version()
	return true
}

func (rc *reqCtx) getServersForEventID(eventID string) (string, gomatrixserverlib.RoomVersion, []gomatrixserverlib.ServerName) {
	if len(rc.authorisedRoomIDs) != 1 {
		util.GetLogger(rc.ctx).WithField("event_id", eventID).Error(
			"getServersForEventID: thread exists over multiple rooms and reached unknown event, cannot determine room and hence which servers to query",
		)
		return "", "", nil
	}
	var roomID string
	var roomVer gomatrixserverlib.RoomVersion
	for r, v := range rc.authorisedRoomIDs {
		roomID = r
		roomVer = v
	}
	var queryRes fs.QueryJoinedHostServerNamesInRoomResponse
	err := rc.fsAPI.QueryJoinedHostServerNamesInRoom(rc.ctx, &fs.QueryJoinedHostServerNamesInRoomRequest{
		RoomID: roomID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("getServersForEventID: failed to QueryJoinedHostServerNamesInRoom")
		return "", "", nil
	}
	// query up to 5 servers
	serversToQuery := queryRes.ServerNames
	if len(serversToQuery) > 5 {
		serversToQuery = serversToQuery[:5]
	}
	return roomID, roomVer, serversToQuery
}

// remoteEvent queries for the event ID given.
func (rc *reqCtx) remoteEvent(eventID string) *gomatrixserverlib.HeaderedEvent {
	if rc.isFederatedRequest {
		return nil // we don't query remote servers for remote requests
	}
	_, roomVer, serversToQuery := rc.getServersForEventID(eventID)
	for _, s := range serversToQuery {
		txn, err := rc.fsAPI.GetEvent(rc.ctx, s, eventID)
		if err != nil {
			util.GetLogger(rc.ctx).WithError(err).Warn("remoteEvent: failed to GetEvent")
			continue
		}
		if len(txn.PDUs) != 1 {
			continue
		}
		ev, err := gomatrixserverlib.NewEventFromUntrustedJSON(txn.PDUs[0], roomVer)
		if err != nil {
			util.GetLogger(rc.ctx).WithError(err).Warn("remoteEvent: failed to NewEventFromUntrustedJSON")
			continue
		}
		// TODO: check sigs on event
		// TODO: check auth events on event
		return ev.Headered(roomVer)
	}
	return nil
}

func (rc *reqCtx) remoteEventRelationships(eventID string) *gomatrixserverlib.MSC2836EventRelationshipsResponse {
	if rc.isFederatedRequest {
		return nil // we don't query remote servers for remote requests
	}
	_, roomVer, serversToQuery := rc.getServersForEventID(eventID)
	var res *gomatrixserverlib.MSC2836EventRelationshipsResponse
	var err error
	for _, srv := range serversToQuery {
		res, err = rc.MSC2836EventRelationships(eventID, srv, roomVer)
		if err != nil {
			util.GetLogger(rc.ctx).WithError(err).WithField("server", srv).Error("remoteEventRelationships: failed to call MSC2836EventRelationships")
		} else {
			break
		}
	}
	return res
}

// lookForEvent returns the event for the event ID given, by trying to auto-join rooms if not authorised and by querying remote servers
// if the event ID is unknown. If `exploreThread` is true, remote requests will use /event_relationships instead of /event. This is
// desirable when walking the thread, but is not desirable when satisfying include_parent|children flags.
func (rc *reqCtx) lookForEvent(eventID string, exploreThread bool) *gomatrixserverlib.HeaderedEvent {
	event := rc.getLocalEvent(eventID)
	if event == nil {
		if exploreThread {
			queryRes := rc.remoteEventRelationships(eventID)
			if queryRes != nil {
				// inject all the events into the roomserver then return the event in question
				rc.injectResponseToRoomserver(queryRes)
				for _, ev := range queryRes.Events {
					if ev.EventID() == eventID {
						return ev.Headered(ev.Version())
					}
				}
			}
			// if we fail to query /event_relationships or we don't have the event queried in the response, fallthrough
			// to do a /event call.
		}
		// this event may have occurred before we joined the room, so delegate to another server to see if they know anything.
		// Only ask for this event though.
		event = rc.remoteEvent(eventID)
		if event == nil {
			return nil
		}
	}
	if rc.authorisedToSeeEvent(event) {
		return event
	}
	if !rc.isFederatedRequest && rc.req.AutoJoin {
		// attempt to join the room then recheck auth, but only for local users
		var joinRes roomserver.PerformJoinResponse
		rc.rsAPI.PerformJoin(rc.ctx, &roomserver.PerformJoinRequest{
			UserID:        rc.userID,
			Content:       map[string]interface{}{},
			RoomIDOrAlias: event.RoomID(),
		}, &joinRes)
		if joinRes.Error != nil {
			util.GetLogger(rc.ctx).WithError(joinRes.Error).WithField("room_id", event.RoomID()).Error("Failed to auto-join room")
			return nil
		}
		delete(rc.authorisedRoomIDs, event.RoomID())
		if rc.authorisedToSeeEvent(event) {
			return event
		}
	}
	return nil
}

func (rc *reqCtx) allowedToSeeEvent(roomID, userID string) (bool, error) {
	// Allow events if the member is in the room
	// TODO: This does not honour history_visibility
	// TODO: This does not honour m.room.create content
	var queryMembershipRes roomserver.QueryMembershipForUserResponse
	err := rc.rsAPI.QueryMembershipForUser(rc.ctx, &roomserver.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: userID,
	}, &queryMembershipRes)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("allowedToSeeEvent: failed to QueryMembershipForUser")
		return false, err
	}
	return queryMembershipRes.IsInRoom, nil
}

func (rc *reqCtx) getLocalEvent(eventID string) *gomatrixserverlib.HeaderedEvent {
	var queryEventsRes roomserver.QueryEventsByIDResponse
	err := rc.rsAPI.QueryEventsByID(rc.ctx, &roomserver.QueryEventsByIDRequest{
		EventIDs: []string{eventID},
	}, &queryEventsRes)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("getLocalEvent: failed to QueryEventsByID")
		return nil
	}
	if len(queryEventsRes.Events) == 0 {
		util.GetLogger(rc.ctx).Infof("getLocalEvent: event does not exist")
		return nil // event does not exist
	}
	return queryEventsRes.Events[0]
}

func (rc *reqCtx) injectResponseToRoomserver(res *gomatrixserverlib.MSC2836EventRelationshipsResponse) {
	var stateEvents []*gomatrixserverlib.Event
	for _, ev := range res.Events {
		if ev.StateKey() != nil {
			stateEvents = append(stateEvents, ev)
		}
	}
	respState := gomatrixserverlib.RespState{
		AuthEvents:  res.AuthChain,
		StateEvents: stateEvents,
	}
	eventsInOrder, err := respState.Events()
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("failed to calculate order to send events in MSC2836EventRelationshipsResponse")
		return
	}
	// everything gets sent as an outlier because auth chain events may be disjoint from the DAG
	// as may the threaded events.
	var ires []roomserver.InputRoomEvent
	for _, outlier := range eventsInOrder {
		ires = append(ires, roomserver.InputRoomEvent{
			Kind:         roomserver.KindOutlier,
			Event:        outlier.Headered(outlier.Version()),
			AuthEventIDs: outlier.AuthEventIDs(),
		})
	}
	// we've got the data by this point so use a background context
	err = roomserver.SendInputRoomEvents(context.Background(), rc.rsAPI, ires)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("failed to inject MSC2836EventRelationshipsResponse into the roomserver")
	}
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
	children, err := w.db.ChildrenForParent(w.ctx, eventID, constRelType, w.req.RecentFirst)
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
		children, err = w.db.ChildrenForParent(w.ctx, next.EventID, constRelType, w.req.RecentFirst)
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

	if w.req.DepthFirst {
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
	if w.req.DepthFirst {
		// toWalk is a stack so pop the child off
		child, toWalk = toWalk[len(toWalk)-1], toWalk[:len(toWalk)-1]
		return &child, toWalk
	}
	// toWalk is a queue so shift the child off
	child, toWalk = toWalk[0], toWalk[1:]
	return &child, toWalk
}
