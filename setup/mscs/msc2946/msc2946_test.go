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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs/msc2836"
	"github.com/matrix-org/dendrite/setup/mscs/msc2946"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

var (
	client = &http.Client{
		Timeout: 10 * time.Second,
	}
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
//                      |
//                      R5
func TestMSC2946(t *testing.T) {
	alice := "@alice:localhost"
	// give access tokens to all three users
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
			"via":     []string{"localhost"},
			"present": true,
		},
	})
	rootToR2 := mustCreateEvent(t, fledglingEvent{
		RoomID:   rootSpace,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &room2,
		Content: map[string]interface{}{
			"via":     []string{"localhost"},
			"present": true,
		},
	})
	rootToS1 := mustCreateEvent(t, fledglingEvent{
		RoomID:   rootSpace,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &subSpaceS1,
		Content: map[string]interface{}{
			"via":     []string{"localhost"},
			"present": true,
		},
	})
	s1ToR3 := mustCreateEvent(t, fledglingEvent{
		RoomID:   subSpaceS1,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &room3,
		Content: map[string]interface{}{
			"via":     []string{"localhost"},
			"present": true,
		},
	})
	s1ToR4 := mustCreateEvent(t, fledglingEvent{
		RoomID:   subSpaceS1,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &room4,
		Content: map[string]interface{}{
			"via":     []string{"localhost"},
			"present": true,
		},
	})
	s1ToS2 := mustCreateEvent(t, fledglingEvent{
		RoomID:   subSpaceS1,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &subSpaceS2,
		Content: map[string]interface{}{
			"via":     []string{"localhost"},
			"present": true,
		},
	})
	s2ToR5 := mustCreateEvent(t, fledglingEvent{
		RoomID:   subSpaceS2,
		Sender:   alice,
		Type:     msc2946.ConstSpaceChildEventType,
		StateKey: &room5,
		Content: map[string]interface{}{
			"via":     []string{"localhost"},
			"present": true,
		},
	})
	nopRsAPI := &testRoomserverAPI{
		userToJoinedRooms: map[string][]string{
			alice: allRooms,
		},
		events: map[string]*gomatrixserverlib.HeaderedEvent{
			rootToR1.EventID(): rootToR1,
			rootToR2.EventID(): rootToR2,
			rootToS1.EventID(): rootToS1,
			s1ToR3.EventID():   s1ToR3,
			s1ToR4.EventID():   s1ToR4,
			s1ToS2.EventID():   s1ToS2,
			s2ToR5.EventID():   s2ToR5,
		},
	}
	router := injectEvents(t, nopUserAPI, nopRsAPI, []*gomatrixserverlib.HeaderedEvent{
		rootToR1, rootToR2, rootToS1,
		s1ToR3, s1ToR4, s1ToS2,
		s2ToR5,
	})
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
}

func newReq(t *testing.T, jsonBody map[string]interface{}) *msc2946.SpacesRequest {
	t.Helper()
	b, err := json.Marshal(jsonBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %s", err)
	}
	var r msc2946.SpacesRequest
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("Failed to unmarshal request: %s", err)
	}
	return &r
}

func runServer(t *testing.T, router *mux.Router) func() {
	t.Helper()
	externalServ := &http.Server{
		Addr:         string(":8009"),
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

func postSpaces(t *testing.T, expectCode int, accessToken, roomID string, req *msc2946.SpacesRequest) *msc2946.SpacesResponse {
	t.Helper()
	var r msc2946.SpacesRequest
	r.Defaults()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %s", err)
	}
	httpReq, err := http.NewRequest(
		"POST", "http://localhost:8009/_matrix/client/unstable/rooms/"+url.PathEscape(roomID)+"/spaces",
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
		var result msc2946.SpacesResponse
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("response 200 OK but failed to read response body: %s", err)
		}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("response 200 OK but failed to deserialise JSON : %s\nbody: %s", err, string(body))
		}
		return &result
	}
	return nil
}

func assertContains(t *testing.T, result *msc2836.EventRelationshipResponse, wantEventIDs []string) {
	t.Helper()
	gotEventIDs := make([]string, len(result.Events))
	for i, ev := range result.Events {
		gotEventIDs[i] = ev.EventID
	}
	if len(gotEventIDs) != len(wantEventIDs) {
		t.Fatalf("length mismatch: got %v want %v", gotEventIDs, wantEventIDs)
	}
	for i := range gotEventIDs {
		if gotEventIDs[i] != wantEventIDs[i] {
			t.Errorf("wrong item in position %d - got %s want %s", i, gotEventIDs[i], wantEventIDs[i])
		}
	}
}

func assertUnsignedChildren(t *testing.T, ev gomatrixserverlib.ClientEvent, relType string, wantCount int, childrenEventIDs []string) {
	t.Helper()
	unsigned := struct {
		Children map[string]int `json:"children"`
		Hash     string         `json:"children_hash"`
	}{}
	if err := json.Unmarshal(ev.Unsigned, &unsigned); err != nil {
		if wantCount == 0 {
			return // no children so possible there is no unsigned field at all
		}
		t.Fatalf("Failed to unmarshal unsigned field: %s", err)
	}
	// zero checks
	if wantCount == 0 {
		if len(unsigned.Children) != 0 || unsigned.Hash != "" {
			t.Fatalf("want 0 children but got unsigned fields %+v", unsigned)
		}
		return
	}
	gotCount := unsigned.Children[relType]
	if gotCount != wantCount {
		t.Errorf("Got %d count, want %d count for rel_type %s", gotCount, wantCount, relType)
	}
	// work out the hash
	sort.Strings(childrenEventIDs)
	var b strings.Builder
	for _, s := range childrenEventIDs {
		b.WriteString(s)
	}
	t.Logf("hashing %s", b.String())
	hashValBytes := sha256.Sum256([]byte(b.String()))
	wantHash := base64.RawStdEncoding.EncodeToString(hashValBytes[:])
	if wantHash != unsigned.Hash {
		t.Errorf("Got unsigned hash %s want hash %s", unsigned.Hash, wantHash)
	}
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
	userToJoinedRooms map[string][]string
	events            map[string]*gomatrixserverlib.HeaderedEvent
}

func (r *testRoomserverAPI) QueryEventsByID(ctx context.Context, req *roomserver.QueryEventsByIDRequest, res *roomserver.QueryEventsByIDResponse) error {
	for _, eventID := range req.EventIDs {
		ev := r.events[eventID]
		if ev != nil {
			res.Events = append(res.Events, ev)
		}
	}
	return nil
}

func (r *testRoomserverAPI) QueryMembershipForUser(ctx context.Context, req *roomserver.QueryMembershipForUserRequest, res *roomserver.QueryMembershipForUserResponse) error {
	rooms := r.userToJoinedRooms[req.UserID]
	for _, roomID := range rooms {
		if roomID == req.RoomID {
			res.IsInRoom = true
			res.HasBeenInRoom = true
			res.Membership = "join"
			break
		}
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

	err := msc2946.Enable(base, rsAPI, userAPI)
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
	roomVer := gomatrixserverlib.RoomVersionV6
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
