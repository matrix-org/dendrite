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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	fs "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi/synctypes"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

const (
	constRelType = "m.reference"
)

type EventRelationshipRequest struct {
	EventID         string `json:"event_id"`
	RoomID          string `json:"room_id"`
	MaxDepth        int    `json:"max_depth"`
	MaxBreadth      int    `json:"max_breadth"`
	Limit           int    `json:"limit"`
	DepthFirst      bool   `json:"depth_first"`
	RecentFirst     bool   `json:"recent_first"`
	IncludeParent   bool   `json:"include_parent"`
	IncludeChildren bool   `json:"include_children"`
	Direction       string `json:"direction"`
	Batch           string `json:"batch"`
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
	Events    []synctypes.ClientEvent `json:"events"`
	NextBatch string                  `json:"next_batch"`
	Limited   bool                    `json:"limited"`
}

type MSC2836EventRelationshipsResponse struct {
	fclient.MSC2836EventRelationshipsResponse
	ParsedEvents    []gomatrixserverlib.PDU
	ParsedAuthChain []gomatrixserverlib.PDU
}

func toClientResponse(ctx context.Context, res *MSC2836EventRelationshipsResponse, rsAPI roomserver.RoomserverInternalAPI) *EventRelationshipResponse {
	out := &EventRelationshipResponse{
		Events: synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(res.ParsedEvents), synctypes.FormatAll, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		}),
		Limited:   res.Limited,
		NextBatch: res.NextBatch,
	}
	return out
}

// Enable this MSC
func Enable(
	cfg *config.Dendrite, cm *sqlutil.Connections, routers httputil.Routers, rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationInternalAPI,
	userAPI userapi.UserInternalAPI, keyRing gomatrixserverlib.JSONVerifier,
) error {
	db, err := NewDatabase(cm, &cfg.MSCs.Database)
	if err != nil {
		return fmt.Errorf("cannot enable MSC2836: %w", err)
	}
	hooks.Enable()
	hooks.Attach(hooks.KindNewEventPersisted, func(headeredEvent interface{}) {
		he := headeredEvent.(*types.HeaderedEvent)
		hookErr := db.StoreRelation(context.Background(), he)
		if hookErr != nil {
			util.GetLogger(context.Background()).WithError(hookErr).WithField("event_id", he.EventID()).Error(
				"failed to StoreRelation",
			)
		}
		// we need to update child metadata here as well as after doing remote /event_relationships requests
		// so we catch child metadata originating from /send transactions
		hookErr = db.UpdateChildMetadata(context.Background(), he)
		if hookErr != nil {
			util.GetLogger(context.Background()).WithError(err).WithField("event_id", he.EventID()).Warn(
				"failed to update child metadata for event",
			)
		}
	})

	routers.Client.Handle("/unstable/event_relationships",
		httputil.MakeAuthAPI("eventRelationships", userAPI, eventRelationshipHandler(db, rsAPI, fsAPI)),
	).Methods(http.MethodPost, http.MethodOptions)

	routers.Federation.Handle("/unstable/event_relationships", httputil.MakeExternalAPI(
		"msc2836_event_relationships", func(req *http.Request) util.JSONResponse {
			fedReq, errResp := fclient.VerifyHTTPRequest(
				req, time.Now(), cfg.Global.ServerName, cfg.Global.IsLocalServerName, keyRing,
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
	ctx         context.Context
	rsAPI       roomserver.RoomserverInternalAPI
	db          Database
	req         *EventRelationshipRequest
	userID      spec.UserID
	roomVersion gomatrixserverlib.RoomVersion

	// federated request args
	isFederatedRequest bool
	serverName         spec.ServerName
	fsAPI              fs.FederationInternalAPI
}

func eventRelationshipHandler(db Database, rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationInternalAPI) func(*http.Request, *userapi.Device) util.JSONResponse {
	return func(req *http.Request, device *userapi.Device) util.JSONResponse {
		relation, err := NewEventRelationshipRequest(req.Body)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("failed to decode HTTP request as JSON")
			return util.JSONResponse{
				Code: 400,
				JSON: spec.BadJSON(fmt.Sprintf("invalid json: %s", err)),
			}
		}
		userID, err := spec.NewUserID(device.UserID, true)
		if err != nil {
			return util.JSONResponse{
				Code: 400,
				JSON: spec.BadJSON(fmt.Sprintf("invalid json: %s", err)),
			}
		}
		rc := reqCtx{
			ctx:                req.Context(),
			req:                relation,
			userID:             *userID,
			rsAPI:              rsAPI,
			fsAPI:              fsAPI,
			isFederatedRequest: false,
			db:                 db,
		}
		res, resErr := rc.process()
		if resErr != nil {
			return *resErr
		}

		return util.JSONResponse{
			Code: 200,
			JSON: toClientResponse(req.Context(), res, rsAPI),
		}
	}
}

func federatedEventRelationship(
	ctx context.Context, fedReq *fclient.FederationRequest, db Database, rsAPI roomserver.RoomserverInternalAPI, fsAPI fs.FederationInternalAPI,
) util.JSONResponse {
	relation, err := NewEventRelationshipRequest(bytes.NewBuffer(fedReq.Content()))
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("failed to decode HTTP request as JSON")
		return util.JSONResponse{
			Code: 400,
			JSON: spec.BadJSON(fmt.Sprintf("invalid json: %s", err)),
		}
	}
	rc := reqCtx{
		ctx:   ctx,
		req:   relation,
		rsAPI: rsAPI,
		db:    db,
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
	for _, ev := range res.ParsedEvents {
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
	res.AuthChain = make(gomatrixserverlib.EventJSONs, len(queryRes.AuthChain))
	for i := range queryRes.AuthChain {
		res.AuthChain[i] = queryRes.AuthChain[i].JSON()
	}

	res.Events = make(gomatrixserverlib.EventJSONs, len(res.ParsedEvents))
	for i := range res.ParsedEvents {
		res.Events[i] = res.ParsedEvents[i].JSON()
	}

	return util.JSONResponse{
		Code: 200,
		JSON: res.MSC2836EventRelationshipsResponse,
	}
}

func (rc *reqCtx) process() (*MSC2836EventRelationshipsResponse, *util.JSONResponse) {
	var res MSC2836EventRelationshipsResponse
	var returnEvents []*types.HeaderedEvent
	// Can the user see (according to history visibility) event_id? If no, reject the request, else continue.
	event := rc.getLocalEvent(rc.req.RoomID, rc.req.EventID)
	if event == nil {
		event = rc.fetchUnknownEvent(rc.req.EventID, rc.req.RoomID)
	}
	if rc.req.RoomID == "" && event != nil {
		rc.req.RoomID = event.RoomID()
	}
	if event == nil || !rc.authorisedToSeeEvent(event) {
		return nil, &util.JSONResponse{
			Code: 403,
			JSON: spec.Forbidden("Event does not exist or you are not authorised to see it"),
		}
	}
	rc.roomVersion = event.Version()

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
		var events []*types.HeaderedEvent
		events, walkLimited = walkThread(
			rc.ctx, rc.db, rc, included, remaining,
		)
		returnEvents = append(returnEvents, events...)
	}
	res.ParsedEvents = make([]gomatrixserverlib.PDU, len(returnEvents))
	for i, ev := range returnEvents {
		// for each event, extract the children_count | hash and add it as unsigned data.
		rc.addChildMetadata(ev)
		res.ParsedEvents[i] = ev.PDU
	}
	res.Limited = remaining == 0 || walkLimited
	return &res, nil
}

// fetchUnknownEvent retrieves an unknown event from the room specified. This server must
// be joined to the room in question. This has the side effect of injecting surround threaded
// events into the roomserver.
func (rc *reqCtx) fetchUnknownEvent(eventID, roomID string) *types.HeaderedEvent {
	if rc.isFederatedRequest || roomID == "" {
		// we don't do fed hits for fed requests, and we can't ask servers without a room ID!
		return nil
	}
	logger := util.GetLogger(rc.ctx).WithField("room_id", roomID)
	// if they supplied a room_id, check the room exists.

	roomVersion, err := rc.rsAPI.QueryRoomVersionForRoom(rc.ctx, roomID)
	if err != nil {
		logger.WithError(err).Warn("failed to query room version for room, does this room exist?")
		return nil
	}

	// check the user is joined to that room
	var queryMemRes roomserver.QueryMembershipForUserResponse
	err = rc.rsAPI.QueryMembershipForUser(rc.ctx, &roomserver.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: rc.userID,
	}, &queryMemRes)
	if err != nil {
		logger.WithError(err).Warn("failed to query membership for user in room")
		return nil
	}
	if !queryMemRes.IsInRoom {
		return nil
	}

	// ask one of the servers in the room for the event
	var queryRes fs.QueryJoinedHostServerNamesInRoomResponse
	err = rc.fsAPI.QueryJoinedHostServerNamesInRoom(rc.ctx, &fs.QueryJoinedHostServerNamesInRoomRequest{
		RoomID: roomID,
	}, &queryRes)
	if err != nil {
		logger.WithError(err).Error("failed to QueryJoinedHostServerNamesInRoom")
		return nil
	}
	// query up to 5 servers
	serversToQuery := queryRes.ServerNames
	if len(serversToQuery) > 5 {
		serversToQuery = serversToQuery[:5]
	}

	// fetch the event, along with some of the surrounding thread (if it's threaded) and the auth chain.
	// Inject the response into the roomserver to remember the event across multiple calls and to set
	// unexplored flags correctly.
	for _, srv := range serversToQuery {
		res, err := rc.MSC2836EventRelationships(eventID, srv, roomVersion)
		if err != nil {
			continue
		}
		rc.injectResponseToRoomserver(res)
		for _, ev := range res.ParsedEvents {
			if ev.EventID() == eventID {
				return &types.HeaderedEvent{PDU: ev}
			}
		}
	}
	logger.WithField("servers", serversToQuery).Warn("failed to query event relationships")
	return nil
}

// If include_parent: true and there is a valid m.relationship field in the event,
// retrieve the referenced event. Apply history visibility check to that event and if it passes, add it to the response array.
func (rc *reqCtx) includeParent(childEvent *types.HeaderedEvent) (parent *types.HeaderedEvent) {
	parentID, _, _ := parentChildEventIDs(childEvent)
	if parentID == "" {
		return nil
	}
	return rc.lookForEvent(parentID)
}

// If include_children: true, lookup all events which have event_id as an m.relationship
// Apply history visibility checks to all these events and add the ones which pass into the response array,
// honouring the recent_first flag and the limit.
func (rc *reqCtx) includeChildren(db Database, parentID string, limit int, recentFirst bool) ([]*types.HeaderedEvent, *util.JSONResponse) {
	if rc.hasUnexploredChildren(parentID) {
		// we need to do a remote request to pull in the children as we are missing them locally.
		serversToQuery := rc.getServersForEventID(parentID)
		var result *MSC2836EventRelationshipsResponse
		for _, srv := range serversToQuery {
			res, err := rc.fsAPI.MSC2836EventRelationships(rc.ctx, rc.serverName, srv, fclient.MSC2836EventRelationshipsRequest{
				EventID:     parentID,
				Direction:   "down",
				Limit:       100,
				MaxBreadth:  -1,
				MaxDepth:    1, // we just want the children from this parent
				RecentFirst: true,
			}, rc.roomVersion)
			if err != nil {
				util.GetLogger(rc.ctx).WithError(err).WithField("server", srv).Error("includeChildren: failed to call MSC2836EventRelationships")
			} else {
				mscRes := &MSC2836EventRelationshipsResponse{
					MSC2836EventRelationshipsResponse: res,
				}
				mscRes.ParsedEvents = res.Events.UntrustedEvents(rc.roomVersion)
				mscRes.ParsedAuthChain = res.AuthChain.UntrustedEvents(rc.roomVersion)
				result = mscRes
				break
			}
		}
		if result != nil {
			rc.injectResponseToRoomserver(result)
		}
		// fallthrough to pull these new events from the DB
	}
	children, err := db.ChildrenForParent(rc.ctx, parentID, constRelType, recentFirst)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("failed to get ChildrenForParent")
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	var childEvents []*types.HeaderedEvent
	for _, child := range children {
		childEvent := rc.lookForEvent(child.EventID)
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
func walkThread(
	ctx context.Context, db Database, rc *reqCtx, included map[string]bool, limit int,
) ([]*types.HeaderedEvent, bool) {
	var result []*types.HeaderedEvent
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
			event := rc.lookForEvent(wi.EventID)
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

// MSC2836EventRelationships performs an /event_relationships request to a remote server
func (rc *reqCtx) MSC2836EventRelationships(eventID string, srv spec.ServerName, ver gomatrixserverlib.RoomVersion) (*MSC2836EventRelationshipsResponse, error) {
	res, err := rc.fsAPI.MSC2836EventRelationships(rc.ctx, rc.serverName, srv, fclient.MSC2836EventRelationshipsRequest{
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
	mscRes := &MSC2836EventRelationshipsResponse{
		MSC2836EventRelationshipsResponse: res,
	}
	mscRes.ParsedEvents = res.Events.UntrustedEvents(ver)
	mscRes.ParsedAuthChain = res.AuthChain.UntrustedEvents(ver)
	return mscRes, nil

}

// authorisedToSeeEvent checks that the user or server is allowed to see this event. Returns true if allowed to
// see this request. This only needs to be done once per room at present as we just check for joined status.
func (rc *reqCtx) authorisedToSeeEvent(event *types.HeaderedEvent) bool {
	if rc.isFederatedRequest {
		// make sure the server is in this room
		var res fs.QueryJoinedHostServerNamesInRoomResponse
		err := rc.fsAPI.QueryJoinedHostServerNamesInRoom(rc.ctx, &fs.QueryJoinedHostServerNamesInRoomRequest{
			RoomID: event.RoomID(),
		}, &res)
		if err != nil {
			util.GetLogger(rc.ctx).WithError(err).Error("authorisedToSeeEvent: failed to QueryJoinedHostServerNamesInRoom")
			return false
		}
		for _, srv := range res.ServerNames {
			if srv == rc.serverName {
				return true
			}
		}
		return false
	}
	// make sure the user is in this room
	// Allow events if the member is in the room
	// TODO: This does not honour history_visibility
	// TODO: This does not honour m.room.create content
	var queryMembershipRes roomserver.QueryMembershipForUserResponse
	err := rc.rsAPI.QueryMembershipForUser(rc.ctx, &roomserver.QueryMembershipForUserRequest{
		RoomID: event.RoomID(),
		UserID: rc.userID,
	}, &queryMembershipRes)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("authorisedToSeeEvent: failed to QueryMembershipForUser")
		return false
	}
	return queryMembershipRes.IsInRoom
}

func (rc *reqCtx) getServersForEventID(eventID string) []spec.ServerName {
	if rc.req.RoomID == "" {
		util.GetLogger(rc.ctx).WithField("event_id", eventID).Error(
			"getServersForEventID: event exists in unknown room",
		)
		return nil
	}
	if rc.roomVersion == "" {
		util.GetLogger(rc.ctx).WithField("event_id", eventID).Errorf(
			"getServersForEventID: event exists in %s with unknown room version", rc.req.RoomID,
		)
		return nil
	}
	var queryRes fs.QueryJoinedHostServerNamesInRoomResponse
	err := rc.fsAPI.QueryJoinedHostServerNamesInRoom(rc.ctx, &fs.QueryJoinedHostServerNamesInRoomRequest{
		RoomID: rc.req.RoomID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("getServersForEventID: failed to QueryJoinedHostServerNamesInRoom")
		return nil
	}
	// query up to 5 servers
	serversToQuery := queryRes.ServerNames
	if len(serversToQuery) > 5 {
		serversToQuery = serversToQuery[:5]
	}
	return serversToQuery
}

func (rc *reqCtx) remoteEventRelationships(eventID string) *MSC2836EventRelationshipsResponse {
	if rc.isFederatedRequest {
		return nil // we don't query remote servers for remote requests
	}
	serversToQuery := rc.getServersForEventID(eventID)
	var res *MSC2836EventRelationshipsResponse
	var err error
	for _, srv := range serversToQuery {
		res, err = rc.MSC2836EventRelationships(eventID, srv, rc.roomVersion)
		if err != nil {
			util.GetLogger(rc.ctx).WithError(err).WithField("server", srv).Error("remoteEventRelationships: failed to call MSC2836EventRelationships")
		} else {
			break
		}
	}
	return res
}

// lookForEvent returns the event for the event ID given, by trying to query remote servers
// if the event ID is unknown via /event_relationships.
func (rc *reqCtx) lookForEvent(eventID string) *types.HeaderedEvent {
	event := rc.getLocalEvent(rc.req.RoomID, eventID)
	if event == nil {
		queryRes := rc.remoteEventRelationships(eventID)
		if queryRes != nil {
			// inject all the events into the roomserver then return the event in question
			rc.injectResponseToRoomserver(queryRes)
			for _, ev := range queryRes.ParsedEvents {
				if ev.EventID() == eventID && rc.req.RoomID == ev.RoomID() {
					return &types.HeaderedEvent{PDU: ev}
				}
			}
		}
	} else if rc.hasUnexploredChildren(eventID) {
		// we have the local event but we may need to do a remote hit anyway if we are exploring the thread and have unknown children.
		// If we don't do this then we risk never fetching the children.
		queryRes := rc.remoteEventRelationships(eventID)
		if queryRes != nil {
			rc.injectResponseToRoomserver(queryRes)
			err := rc.db.MarkChildrenExplored(context.Background(), eventID)
			if err != nil {
				util.GetLogger(rc.ctx).WithError(err).Warnf("failed to mark children of %s as explored", eventID)
			}
		}
	}
	if rc.req.RoomID == event.RoomID() {
		return event
	}
	return nil
}

func (rc *reqCtx) getLocalEvent(roomID, eventID string) *types.HeaderedEvent {
	var queryEventsRes roomserver.QueryEventsByIDResponse
	err := rc.rsAPI.QueryEventsByID(rc.ctx, &roomserver.QueryEventsByIDRequest{
		RoomID:   roomID,
		EventIDs: []string{eventID},
	}, &queryEventsRes)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("getLocalEvent: failed to QueryEventsByID")
		return nil
	}
	if len(queryEventsRes.Events) == 0 {
		util.GetLogger(rc.ctx).WithField("event_id", eventID).Infof("getLocalEvent: event does not exist")
		return nil // event does not exist
	}
	return queryEventsRes.Events[0]
}

// injectResponseToRoomserver injects the events
// into the roomserver as KindOutlier, with auth chains.
func (rc *reqCtx) injectResponseToRoomserver(res *MSC2836EventRelationshipsResponse) {
	var stateEvents gomatrixserverlib.EventJSONs
	var messageEvents []gomatrixserverlib.PDU
	for _, ev := range res.ParsedEvents {
		if ev.StateKey() != nil {
			stateEvents = append(stateEvents, ev.JSON())
		} else {
			messageEvents = append(messageEvents, ev)
		}
	}
	respState := &fclient.RespState{
		AuthEvents:  res.AuthChain,
		StateEvents: stateEvents,
	}
	eventsInOrder := gomatrixserverlib.LineariseStateResponse(rc.roomVersion, respState)
	// everything gets sent as an outlier because auth chain events may be disjoint from the DAG
	// as may the threaded events.
	var ires []roomserver.InputRoomEvent
	for _, outlier := range append(eventsInOrder, messageEvents...) {
		ires = append(ires, roomserver.InputRoomEvent{
			Kind:  roomserver.KindOutlier,
			Event: &types.HeaderedEvent{PDU: outlier},
		})
	}
	// we've got the data by this point so use a background context
	err := roomserver.SendInputRoomEvents(context.Background(), rc.rsAPI, rc.serverName, ires, false)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Error("failed to inject MSC2836EventRelationshipsResponse into the roomserver")
	}
	// update the child count / hash columns for these nodes. We need to do this here because not all events will make it
	// through to the KindNewEventPersisted hook because the roomserver will ignore duplicates. Duplicates have meaning though
	// as the `unsigned` field may differ (if the number of children changes).
	for _, ev := range ires {
		err = rc.db.UpdateChildMetadata(context.Background(), ev.Event)
		if err != nil {
			util.GetLogger(rc.ctx).WithError(err).WithField("event_id", ev.Event.EventID()).Warn("failed to update child metadata for event")
		}
	}
}

func (rc *reqCtx) addChildMetadata(ev *types.HeaderedEvent) {
	count, hash := rc.getChildMetadata(ev.EventID())
	if count == 0 {
		return
	}
	err := ev.SetUnsignedField("children_hash", spec.Base64Bytes(hash))
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Warn("Failed to set children_hash")
	}
	err = ev.SetUnsignedField("children", map[string]int{
		constRelType: count,
	})
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Warn("Failed to set children count")
	}
}

func (rc *reqCtx) getChildMetadata(eventID string) (count int, hash []byte) {
	children, err := rc.db.ChildrenForParent(rc.ctx, eventID, constRelType, false)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).Warn("Failed to get ChildrenForParent for getting child metadata")
		return
	}
	if len(children) == 0 {
		return
	}
	// sort it lexiographically
	sort.Slice(children, func(i, j int) bool {
		return children[i].EventID < children[j].EventID
	})
	// hash it
	var eventIDs strings.Builder
	for _, c := range children {
		_, _ = eventIDs.WriteString(c.EventID)
	}
	hashValBytes := sha256.Sum256([]byte(eventIDs.String()))

	count = len(children)
	hash = hashValBytes[:]
	return
}

// hasUnexploredChildren returns true if this event has unexplored children.
// "An event has unexplored children if the `unsigned` child count on the parent does not match
// how many children the server believes the parent to have. In addition, if the counts match but
// the hashes do not match, then the event is unexplored."
func (rc *reqCtx) hasUnexploredChildren(eventID string) bool {
	if rc.isFederatedRequest {
		return false // we only explore children for clients, not servers.
	}
	// extract largest child count from event
	eventCount, eventHash, explored, err := rc.db.ChildMetadata(rc.ctx, eventID)
	if err != nil {
		util.GetLogger(rc.ctx).WithError(err).WithField("event_id", eventID).Warn(
			"failed to get ChildMetadata from db",
		)
		return false
	}
	// if there are no recorded children then we know we have >= children.
	// if the event has already been explored (read: we hit /event_relationships successfully)
	// then don't do it again. We'll only re-do this if we get an even bigger children count,
	// see Database.UpdateChildMetadata
	if eventCount == 0 || explored {
		return false // short-circuit
	}

	// calculate child count for event
	calcCount, calcHash := rc.getChildMetadata(eventID)

	if eventCount < calcCount {
		return false // we have more children
	} else if eventCount > calcCount {
		return true // the event has more children than we know about
	}
	// we have the same count, so a mismatched hash means some children are different
	return !bytes.Equal(eventHash, calcHash)
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
	children, err := w.childrenForParent(eventID)
	if err != nil {
		util.GetLogger(w.ctx).WithError(err).Error("WalkFrom() childrenForParent failed, cannot walk")
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
		children, err = w.childrenForParent(next.EventID)
		if err != nil {
			util.GetLogger(w.ctx).WithError(err).Error("WalkFrom() childrenForParent failed, cannot walk")
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

// childrenForParent returns the children events for this event ID, honouring the direction: up|down flags
// meaning this can actually be returning the parent for the event instead of the children.
func (w *walker) childrenForParent(eventID string) ([]eventInfo, error) {
	if w.req.Direction == "down" {
		return w.db.ChildrenForParent(w.ctx, eventID, constRelType, w.req.RecentFirst)
	}
	// find the event to pull out the parent
	ei, err := w.db.ParentForChild(w.ctx, eventID, constRelType)
	if err != nil {
		return nil, err
	}
	if ei != nil {
		return []eventInfo{*ei}, nil
	}
	return nil, nil
}
