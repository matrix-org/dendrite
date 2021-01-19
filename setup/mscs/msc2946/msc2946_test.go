// Copyright 2021 The Matrix.org Foundation C.I.C.
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

package msc2946_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs/msc2946"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	client = &http.Client{
		Timeout: 10 * time.Second,
	}
	roomVer = gomatrixserverlib.RoomVersionV6
)

// Basic sanity check of MSC2946 logic. Tests a single room with a few state events
// and a bit of recursion to subspaces. Makes a graph like:
//      Root
//    ____|_____
//   |    |    |
//  R1   R2    S1
//             |_________
//             |    |   |
//             R3   R4  S2
//                      |  <-- this link is just a parent, not a child
//                      R5
//
// Alice is not joined to R4, but R4 is "world_readable".
func TestMSC2946(t *testing.T) {
	alice := "@alice:localhost"
	// give access token to alice
	nopUserAPI := &testUserAPI{
		accessTokens: make(map[string]userapi.Device),
	}
	nopUserAPI.accessTokens["alice"] = userapi.Device{
		AccessToken: "alice",
		DisplayName: "Alice",
		UserID:      alice,
	}
	rootSpace := "!rootspace:localhost"
	subSpaceS1 := "!subspaceS1:localhost"
	subSpaceS2 := "!subspaceS2:localhost"
	room1 := "!room1:localhost"
	room2 := "!room2:localhost"
	room3 := "!room3:localhost"
	room4 := "!room4:localhost"
	empty := ""
	room5 := "!room5:localhost"
	allRooms := []string{
		rootSpace, subSpaceS1, subSpaceS2,
		room1, room2, room3, room4, room5,
	}
	rootToR1 := mustCreateEvent(t, fledglingEvent{
		RoomID:   rootSpace,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &room1,
		Content: map[string]interface{}{
			"via": []string{"localhost"},
		},
	})
	rootToR2 := mustCreateEvent(t, fledglingEvent{
		RoomID:   rootSpace,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &room2,
		Content: map[string]interface{}{
			"via": []string{"localhost"},
		},
	})
	rootToS1 := mustCreateEvent(t, fledglingEvent{
		RoomID:   rootSpace,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &subSpaceS1,
		Content: map[string]interface{}{
			"via": []string{"localhost"},
		},
	})
	s1ToR3 := mustCreateEvent(t, fledglingEvent{
		RoomID:   subSpaceS1,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &room3,
		Content: map[string]interface{}{
			"via": []string{"localhost"},
		},
	})
	s1ToR4 := mustCreateEvent(t, fledglingEvent{
		RoomID:   subSpaceS1,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &room4,
		Content: map[string]interface{}{
			"via": []string{"localhost"},
		},
	})
	s1ToS2 := mustCreateEvent(t, fledglingEvent{
		RoomID:   subSpaceS1,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &subSpaceS2,
		Content: map[string]interface{}{
			"via": []string{"localhost"},
		},
	})
	// This is a parent link only
	s2ToR5 := mustCreateEvent(t, fledglingEvent{
		RoomID:   room5,
		Sender:   alice,
		Type:     msc2946.ConstSpaceParentEventType,
		StateKey: &subSpaceS2,
		Content: map[string]interface{}{
			"via": []string{"localhost"},
		},
	})
	// history visibility for R4
	r4HisVis := mustCreateEvent(t, fledglingEvent{
		RoomID:   room4,
		Sender:   "@someone:localhost",
		Type:     gomatrixserverlib.MRoomHistoryVisibility,
		StateKey: &empty,
		Content: map[string]interface{}{
			"history_visibility": "world_readable",
		},
	})
	var joinEvents []*gomatrixserverlib.HeaderedEvent
	for _, roomID := range allRooms {
		if roomID == room4 {
			continue // not joined to that room
		}
		joinEvents = append(joinEvents, mustCreateEvent(t, fledglingEvent{
			RoomID:   roomID,
			Sender:   alice,
			StateKey: &alice,
			Type:     gomatrixserverlib.MRoomMember,
			Content: map[string]interface{}{
				"membership": "join",
			},
		}))
	}
	roomNameTuple := gomatrixserverlib.StateKeyTuple{
		EventType: "m.room.name",
		StateKey:  "",
	}
	hisVisTuple := gomatrixserverlib.StateKeyTuple{
		EventType: "m.room.history_visibility",
		StateKey:  "",
	}
	nopRsAPI := &testRoomserverAPI{
		joinEvents: joinEvents,
		events: map[string]*gomatrixserverlib.HeaderedEvent{
			rootToR1.EventID(): rootToR1,
			rootToR2.EventID(): rootToR2,
			rootToS1.EventID(): rootToS1,
			s1ToR3.EventID():   s1ToR3,
			s1ToR4.EventID():   s1ToR4,
			s1ToS2.EventID():   s1ToS2,
			s2ToR5.EventID():   s2ToR5,
			r4HisVis.EventID(): r4HisVis,
		},
		pubRoomState: map[string]map[gomatrixserverlib.StateKeyTuple]string{
			rootSpace: {
				roomNameTuple: "Root",
				hisVisTuple:   "shared",
			},
			subSpaceS1: {
				roomNameTuple: "Sub-Space 1",
				hisVisTuple:   "joined",
			},
			subSpaceS2: {
				roomNameTuple: "Sub-Space 2",
				hisVisTuple:   "shared",
			},
			room1: {
				hisVisTuple: "joined",
			},
			room2: {
				hisVisTuple: "joined",
			},
			room3: {
				hisVisTuple: "joined",
			},
			room4: {
				hisVisTuple: "world_readable",
			},
			room5: {
				hisVisTuple: "joined",
			},
		},
	}
	allEvents := []*gomatrixserverlib.HeaderedEvent{
		rootToR1, rootToR2, rootToS1,
		s1ToR3, s1ToR4, s1ToS2,
		s2ToR5, r4HisVis,
	}
	allEvents = append(allEvents, joinEvents...)
	router := injectEvents(t, nopUserAPI, nopRsAPI, allEvents)
	cancel := runServer(t, router)
	defer cancel()

	t.Run("returns no events for unknown rooms", func(t *testing.T) {
		res := postSpaces(t, 200, "alice", "!unknown:localhost", newReq(t, map[string]interface{}{}))
		if len(res.Events) > 0 {
			t.Errorf("got %d events, want 0", len(res.Events))
		}
		if len(res.Rooms) > 0 {
			t.Errorf("got %d rooms, want 0", len(res.Rooms))
		}
	})
	t.Run("returns the entire graph", func(t *testing.T) {
		res := postSpaces(t, 200, "alice", rootSpace, newReq(t, map[string]interface{}{}))
		if len(res.Events) != 7 {
			t.Errorf("got %d events, want 7", len(res.Events))
		}
		if len(res.Rooms) != len(allRooms) {
			t.Errorf("got %d rooms, want %d", len(res.Rooms), len(allRooms))
		}
	})
	t.Run("can update the graph", func(t *testing.T) {
		// remove R3 from the graph
		rmS1ToR3 := mustCreateEvent(t, fledglingEvent{
			RoomID:   subSpaceS1,
			Sender:   alice,
			Type:     msc2946.ConstSpaceChildEventType,
			StateKey: &room3,
			Content:  map[string]interface{}{}, // redacted
		})
		nopRsAPI.events[rmS1ToR3.EventID()] = rmS1ToR3
		hooks.Run(hooks.KindNewEventPersisted, rmS1ToR3)

		res := postSpaces(t, 200, "alice", rootSpace, newReq(t, map[string]interface{}{}))
		if len(res.Events) != 6 { // one less since we don't return redacted events
			t.Errorf("got %d events, want 6", len(res.Events))
		}
		if len(res.Rooms) != (len(allRooms) - 1) { // one less due to lack of R3
			t.Errorf("got %d rooms, want %d", len(res.Rooms), len(allRooms)-1)
		}
	})
}

func newReq(t *testing.T, jsonBody map[string]interface{}) *gomatrixserverlib.MSC2946SpacesRequest {
	t.Helper()
	b, err := json.Marshal(jsonBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %s", err)
	}
	var r gomatrixserverlib.MSC2946SpacesRequest
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("Failed to unmarshal request: %s", err)
	}
	return &r
}

func runServer(t *testing.T, router *mux.Router) func() {
	t.Helper()
	externalServ := &http.Server{
		Addr:         string(":8010"),
		WriteTimeout: 60 * time.Second,
		Handler:      router,
	}
	go func() {
		externalServ.ListenAndServe()
	}()
	// wait to listen on the port
	time.Sleep(500 * time.Millisecond)
	return func() {
		externalServ.Shutdown(context.TODO())
	}
}

func postSpaces(t *testing.T, expectCode int, accessToken, roomID string, req *gomatrixserverlib.MSC2946SpacesRequest) *gomatrixserverlib.MSC2946SpacesResponse {
	t.Helper()
	var r gomatrixserverlib.MSC2946SpacesRequest
	msc2946.Defaults(&r)
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %s", err)
	}
	httpReq, err := http.NewRequest(
		"POST", "http://localhost:8010/_matrix/client/unstable/rooms/"+url.PathEscape(roomID)+"/spaces",
		bytes.NewBuffer(data),
	)
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	if err != nil {
		t.Fatalf("failed to prepare request: %s", err)
	}
	res, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("failed to do request: %s", err)
	}
	if res.StatusCode != expectCode {
		body, _ := ioutil.ReadAll(res.Body)
		t.Fatalf("wrong response code, got %d want %d - body: %s", res.StatusCode, expectCode, string(body))
	}
	if res.StatusCode == 200 {
		var result gomatrixserverlib.MSC2946SpacesResponse
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("response 200 OK but failed to read response body: %s", err)
		}
		t.Logf("Body: %s", string(body))
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("response 200 OK but failed to deserialise JSON : %s\nbody: %s", err, string(body))
		}
		return &result
	}
	return nil
}

type testUserAPI struct {
	accessTokens map[string]userapi.Device
}

func (u *testUserAPI) InputAccountData(ctx context.Context, req *userapi.InputAccountDataRequest, res *userapi.InputAccountDataResponse) error {
	return nil
}
func (u *testUserAPI) PerformAccountCreation(ctx context.Context, req *userapi.PerformAccountCreationRequest, res *userapi.PerformAccountCreationResponse) error {
	return nil
}
func (u *testUserAPI) PerformPasswordUpdate(ctx context.Context, req *userapi.PerformPasswordUpdateRequest, res *userapi.PerformPasswordUpdateResponse) error {
	return nil
}
func (u *testUserAPI) PerformDeviceCreation(ctx context.Context, req *userapi.PerformDeviceCreationRequest, res *userapi.PerformDeviceCreationResponse) error {
	return nil
}
func (u *testUserAPI) PerformDeviceDeletion(ctx context.Context, req *userapi.PerformDeviceDeletionRequest, res *userapi.PerformDeviceDeletionResponse) error {
	return nil
}
func (u *testUserAPI) PerformDeviceUpdate(ctx context.Context, req *userapi.PerformDeviceUpdateRequest, res *userapi.PerformDeviceUpdateResponse) error {
	return nil
}
func (u *testUserAPI) PerformLastSeenUpdate(ctx context.Context, req *userapi.PerformLastSeenUpdateRequest, res *userapi.PerformLastSeenUpdateResponse) error {
	return nil
}
func (u *testUserAPI) PerformAccountDeactivation(ctx context.Context, req *userapi.PerformAccountDeactivationRequest, res *userapi.PerformAccountDeactivationResponse) error {
	return nil
}
func (u *testUserAPI) QueryProfile(ctx context.Context, req *userapi.QueryProfileRequest, res *userapi.QueryProfileResponse) error {
	return nil
}
func (u *testUserAPI) QueryAccessToken(ctx context.Context, req *userapi.QueryAccessTokenRequest, res *userapi.QueryAccessTokenResponse) error {
	dev, ok := u.accessTokens[req.AccessToken]
	if !ok {
		res.Err = fmt.Errorf("unknown token")
		return nil
	}
	res.Device = &dev
	return nil
}
func (u *testUserAPI) QueryDevices(ctx context.Context, req *userapi.QueryDevicesRequest, res *userapi.QueryDevicesResponse) error {
	return nil
}
func (u *testUserAPI) QueryAccountData(ctx context.Context, req *userapi.QueryAccountDataRequest, res *userapi.QueryAccountDataResponse) error {
	return nil
}
func (u *testUserAPI) QueryDeviceInfos(ctx context.Context, req *userapi.QueryDeviceInfosRequest, res *userapi.QueryDeviceInfosResponse) error {
	return nil
}
func (u *testUserAPI) QuerySearchProfiles(ctx context.Context, req *userapi.QuerySearchProfilesRequest, res *userapi.QuerySearchProfilesResponse) error {
	return nil
}

type testRoomserverAPI struct {
	// use a trace API as it implements method stubs so we don't need to have them here.
	// We'll override the functions we care about.
	roomserver.RoomserverInternalAPITrace
	joinEvents   []*gomatrixserverlib.HeaderedEvent
	events       map[string]*gomatrixserverlib.HeaderedEvent
	pubRoomState map[string]map[gomatrixserverlib.StateKeyTuple]string
}

func (r *testRoomserverAPI) QueryServerJoinedToRoom(ctx context.Context, req *roomserver.QueryServerJoinedToRoomRequest, res *roomserver.QueryServerJoinedToRoomResponse) error {
	res.IsInRoom = true
	res.RoomExists = true
	return nil
}

func (r *testRoomserverAPI) QueryBulkStateContent(ctx context.Context, req *roomserver.QueryBulkStateContentRequest, res *roomserver.QueryBulkStateContentResponse) error {
	res.Rooms = make(map[string]map[gomatrixserverlib.StateKeyTuple]string)
	for _, roomID := range req.RoomIDs {
		pubRoomData, ok := r.pubRoomState[roomID]
		if ok {
			res.Rooms[roomID] = pubRoomData
		}
	}
	return nil
}

func (r *testRoomserverAPI) QueryCurrentState(ctx context.Context, req *roomserver.QueryCurrentStateRequest, res *roomserver.QueryCurrentStateResponse) error {
	res.StateEvents = make(map[gomatrixserverlib.StateKeyTuple]*gomatrixserverlib.HeaderedEvent)
	checkEvent := func(he *gomatrixserverlib.HeaderedEvent) {
		if he.RoomID() != req.RoomID {
			return
		}
		if he.StateKey() == nil {
			return
		}
		tuple := gomatrixserverlib.StateKeyTuple{
			EventType: he.Type(),
			StateKey:  *he.StateKey(),
		}
		for _, t := range req.StateTuples {
			if t == tuple {
				res.StateEvents[t] = he
			}
		}
	}
	for _, he := range r.joinEvents {
		checkEvent(he)
	}
	for _, he := range r.events {
		checkEvent(he)
	}
	return nil
}

func injectEvents(t *testing.T, userAPI userapi.UserInternalAPI, rsAPI roomserver.RoomserverInternalAPI, events []*gomatrixserverlib.HeaderedEvent) *mux.Router {
	t.Helper()
	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Global.ServerName = "localhost"
	cfg.MSCs.Database.ConnectionString = "file:msc2946_test.db"
	cfg.MSCs.MSCs = []string{"msc2946"}
	base := &setup.BaseDendrite{
		Cfg:                    cfg,
		PublicClientAPIMux:     mux.NewRouter().PathPrefix(httputil.PublicClientPathPrefix).Subrouter(),
		PublicFederationAPIMux: mux.NewRouter().PathPrefix(httputil.PublicFederationPathPrefix).Subrouter(),
	}

	err := msc2946.Enable(base, rsAPI, userAPI, nil, nil)
	if err != nil {
		t.Fatalf("failed to enable MSC2946: %s", err)
	}
	for _, ev := range events {
		hooks.Run(hooks.KindNewEventPersisted, ev)
	}
	return base.PublicClientAPIMux
}

type fledglingEvent struct {
	Type     string
	StateKey *string
	Content  interface{}
	Sender   string
	RoomID   string
}

func mustCreateEvent(t *testing.T, ev fledglingEvent) (result *gomatrixserverlib.HeaderedEvent) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize) // zero seed
	key := ed25519.NewKeyFromSeed(seed)
	eb := gomatrixserverlib.EventBuilder{
		Sender:   ev.Sender,
		Depth:    999,
		Type:     ev.Type,
		StateKey: ev.StateKey,
		RoomID:   ev.RoomID,
	}
	err := eb.SetContent(ev.Content)
	if err != nil {
		t.Fatalf("mustCreateEvent: failed to marshal event content %+v", ev.Content)
	}
	// make sure the origin_server_ts changes so we can test recency
	time.Sleep(1 * time.Millisecond)
	signedEvent, err := eb.Build(time.Now(), gomatrixserverlib.ServerName("localhost"), "ed25519:test", key, roomVer)
	if err != nil {
		t.Fatalf("mustCreateEvent: failed to sign event: %s", err)
	}
	h := signedEvent.Headered(roomVer)
	return h
}
