package msc2836_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs/msc2836"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

var (
	client = &http.Client{
		Timeout: 10 * time.Second,
	}
)

// Basic sanity check of MSC2836 logic. Injects a thread that looks like:
//
//	  A
//	  |
//	  B
//	 / \
//	C   D
//	   /|\
//	  E F G
//	  |
//	  H
//
// And makes sure POST /event_relationships works with various parameters
func TestMSC2836(t *testing.T) {
	alice := "@alice:localhost"
	bob := "@bob:localhost"
	charlie := "@charlie:localhost"
	roomID := "!alice:localhost"
	// give access tokens to all three users
	nopUserAPI := &testUserAPI{
		accessTokens: make(map[string]userapi.Device),
	}
	nopUserAPI.accessTokens["alice"] = userapi.Device{
		AccessToken: "alice",
		DisplayName: "Alice",
		UserID:      alice,
	}
	nopUserAPI.accessTokens["bob"] = userapi.Device{
		AccessToken: "bob",
		DisplayName: "Bob",
		UserID:      bob,
	}
	nopUserAPI.accessTokens["charlie"] = userapi.Device{
		AccessToken: "charlie",
		DisplayName: "Charles",
		UserID:      charlie,
	}
	eventA := mustCreateEvent(t, fledglingEvent{
		RoomID: roomID,
		Sender: alice,
		Type:   "m.room.message",
		Content: map[string]interface{}{
			"body": "[A] Do you know shelties?",
		},
	})
	eventB := mustCreateEvent(t, fledglingEvent{
		RoomID: roomID,
		Sender: bob,
		Type:   "m.room.message",
		Content: map[string]interface{}{
			"body": "[B] I <3 shelties",
			"m.relationship": map[string]string{
				"rel_type": "m.reference",
				"event_id": eventA.EventID(),
			},
		},
	})
	eventC := mustCreateEvent(t, fledglingEvent{
		RoomID: roomID,
		Sender: bob,
		Type:   "m.room.message",
		Content: map[string]interface{}{
			"body": "[C] like so much",
			"m.relationship": map[string]string{
				"rel_type": "m.reference",
				"event_id": eventB.EventID(),
			},
		},
	})
	eventD := mustCreateEvent(t, fledglingEvent{
		RoomID: roomID,
		Sender: alice,
		Type:   "m.room.message",
		Content: map[string]interface{}{
			"body": "[D] but what are shelties???",
			"m.relationship": map[string]string{
				"rel_type": "m.reference",
				"event_id": eventB.EventID(),
			},
		},
	})
	eventE := mustCreateEvent(t, fledglingEvent{
		RoomID: roomID,
		Sender: bob,
		Type:   "m.room.message",
		Content: map[string]interface{}{
			"body": "[E] seriously???",
			"m.relationship": map[string]string{
				"rel_type": "m.reference",
				"event_id": eventD.EventID(),
			},
		},
	})
	eventF := mustCreateEvent(t, fledglingEvent{
		RoomID: roomID,
		Sender: charlie,
		Type:   "m.room.message",
		Content: map[string]interface{}{
			"body": "[F] omg how do you not know what shelties are",
			"m.relationship": map[string]string{
				"rel_type": "m.reference",
				"event_id": eventD.EventID(),
			},
		},
	})
	eventG := mustCreateEvent(t, fledglingEvent{
		RoomID: roomID,
		Sender: alice,
		Type:   "m.room.message",
		Content: map[string]interface{}{
			"body": "[G] looked it up, it's a sheltered person?",
			"m.relationship": map[string]string{
				"rel_type": "m.reference",
				"event_id": eventD.EventID(),
			},
		},
	})
	eventH := mustCreateEvent(t, fledglingEvent{
		RoomID: roomID,
		Sender: bob,
		Type:   "m.room.message",
		Content: map[string]interface{}{
			"body": "[H] it's a dog!!!!!",
			"m.relationship": map[string]string{
				"rel_type": "m.reference",
				"event_id": eventE.EventID(),
			},
		},
	})
	// make everyone joined to each other's rooms
	nopRsAPI := &testRoomserverAPI{
		userToJoinedRooms: map[string][]string{
			alice:   {roomID},
			bob:     {roomID},
			charlie: {roomID},
		},
		events: map[string]*gomatrixserverlib.HeaderedEvent{
			eventA.EventID(): eventA,
			eventB.EventID(): eventB,
			eventC.EventID(): eventC,
			eventD.EventID(): eventD,
			eventE.EventID(): eventE,
			eventF.EventID(): eventF,
			eventG.EventID(): eventG,
			eventH.EventID(): eventH,
		},
	}
	router := injectEvents(t, nopUserAPI, nopRsAPI, []*gomatrixserverlib.HeaderedEvent{
		eventA, eventB, eventC, eventD, eventE, eventF, eventG, eventH,
	})
	cancel := runServer(t, router)
	defer cancel()

	t.Run("returns 403 on invalid event IDs", func(t *testing.T) {
		_ = postRelationships(t, 403, "alice", newReq(t, map[string]interface{}{
			"event_id": "$invalid",
		}))
	})
	t.Run("returns 403 if not joined to the room of specified event in request", func(t *testing.T) {
		nopUserAPI.accessTokens["frank"] = userapi.Device{
			AccessToken: "frank",
			DisplayName: "Frank Not In Room",
			UserID:      "@frank:localhost",
		}
		_ = postRelationships(t, 403, "frank", newReq(t, map[string]interface{}{
			"event_id":       eventB.EventID(),
			"limit":          1,
			"include_parent": true,
		}))
	})
	t.Run("returns the parent if include_parent is true", func(t *testing.T) {
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":       eventB.EventID(),
			"include_parent": true,
			"limit":          2,
		}))
		assertContains(t, body, []string{eventB.EventID(), eventA.EventID()})
	})
	t.Run("returns the children in the right order if include_children is true", func(t *testing.T) {
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":         eventD.EventID(),
			"include_children": true,
			"recent_first":     true,
			"limit":            4,
		}))
		assertContains(t, body, []string{eventD.EventID(), eventG.EventID(), eventF.EventID(), eventE.EventID()})
		body = postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":         eventD.EventID(),
			"include_children": true,
			"recent_first":     false,
			"limit":            4,
		}))
		assertContains(t, body, []string{eventD.EventID(), eventE.EventID(), eventF.EventID(), eventG.EventID()})
	})
	t.Run("walks the graph depth first", func(t *testing.T) {
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventB.EventID(),
			"recent_first": false,
			"depth_first":  true,
			"limit":        6,
		}))
		// Oldest first so:
		//   A
		//   |
		//   B1
		//  / \
		// C2  D3
		//    /| \
		//  4E 6F G
		//   |
		//  5H
		assertContains(t, body, []string{eventB.EventID(), eventC.EventID(), eventD.EventID(), eventE.EventID(), eventH.EventID(), eventF.EventID()})
		body = postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventB.EventID(),
			"recent_first": true,
			"depth_first":  true,
			"limit":        6,
		}))
		// Recent first so:
		//   A
		//   |
		//   B1
		//  / \
		// C   D2
		//    /| \
		//  E5 F4 G3
		//   |
		//  H6
		assertContains(t, body, []string{eventB.EventID(), eventD.EventID(), eventG.EventID(), eventF.EventID(), eventE.EventID(), eventH.EventID()})
	})
	t.Run("walks the graph breadth first", func(t *testing.T) {
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventB.EventID(),
			"recent_first": false,
			"depth_first":  false,
			"limit":        6,
		}))
		// Oldest first so:
		//   A
		//   |
		//   B1
		//  / \
		// C2  D3
		//    /| \
		//  E4 F5 G6
		//   |
		//   H
		assertContains(t, body, []string{eventB.EventID(), eventC.EventID(), eventD.EventID(), eventE.EventID(), eventF.EventID(), eventG.EventID()})
		body = postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventB.EventID(),
			"recent_first": true,
			"depth_first":  false,
			"limit":        6,
		}))
		// Recent first so:
		//   A
		//   |
		//   B1
		//  / \
		// C3  D2
		//    /| \
		//  E6 F5 G4
		//   |
		//   H
		assertContains(t, body, []string{eventB.EventID(), eventD.EventID(), eventC.EventID(), eventG.EventID(), eventF.EventID(), eventE.EventID()})
	})
	t.Run("caps via max_breadth", func(t *testing.T) {
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventB.EventID(),
			"recent_first": false,
			"depth_first":  false,
			"max_breadth":  2,
			"limit":        10,
		}))
		// Event G gets omitted because of max_breadth
		assertContains(t, body, []string{eventB.EventID(), eventC.EventID(), eventD.EventID(), eventE.EventID(), eventF.EventID(), eventH.EventID()})
	})
	t.Run("caps via max_depth", func(t *testing.T) {
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventB.EventID(),
			"recent_first": false,
			"depth_first":  false,
			"max_depth":    2,
			"limit":        10,
		}))
		// Event H gets omitted because of max_depth
		assertContains(t, body, []string{eventB.EventID(), eventC.EventID(), eventD.EventID(), eventE.EventID(), eventF.EventID(), eventG.EventID()})
	})
	t.Run("terminates when reaching the limit", func(t *testing.T) {
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventB.EventID(),
			"recent_first": false,
			"depth_first":  false,
			"limit":        4,
		}))
		assertContains(t, body, []string{eventB.EventID(), eventC.EventID(), eventD.EventID(), eventE.EventID()})
	})
	t.Run("returns all events with a high enough limit", func(t *testing.T) {
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventB.EventID(),
			"recent_first": false,
			"depth_first":  false,
			"limit":        400,
		}))
		assertContains(t, body, []string{eventB.EventID(), eventC.EventID(), eventD.EventID(), eventE.EventID(), eventF.EventID(), eventG.EventID(), eventH.EventID()})
	})
	t.Run("can navigate up the graph with direction: up", func(t *testing.T) {
		//   A4
		//   |
		//   B3
		//  / \
		// C   D2
		//    /| \
		//   E F1 G
		//   |
		//   H
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventF.EventID(),
			"recent_first": false,
			"depth_first":  true,
			"direction":    "up",
		}))
		assertContains(t, body, []string{eventF.EventID(), eventD.EventID(), eventB.EventID(), eventA.EventID()})
	})
	t.Run("includes children and children_hash in unsigned", func(t *testing.T) {
		body := postRelationships(t, 200, "alice", newReq(t, map[string]interface{}{
			"event_id":     eventB.EventID(),
			"recent_first": false,
			"depth_first":  false,
			"limit":        3,
		}))
		// event B has C,D as children
		// event C has no children
		// event D has 3 children (not included in response)
		assertContains(t, body, []string{eventB.EventID(), eventC.EventID(), eventD.EventID()})
		assertUnsignedChildren(t, body.Events[0], "m.reference", 2, []string{eventC.EventID(), eventD.EventID()})
		assertUnsignedChildren(t, body.Events[1], "", 0, nil)
		assertUnsignedChildren(t, body.Events[2], "m.reference", 3, []string{eventE.EventID(), eventF.EventID(), eventG.EventID()})
	})
}

// TODO: TestMSC2836TerminatesLoops (short and long)
// TODO: TestMSC2836UnknownEventsSkipped
// TODO: TestMSC2836SkipEventIfNotInRoom

func newReq(t *testing.T, jsonBody map[string]interface{}) *msc2836.EventRelationshipRequest {
	t.Helper()
	b, err := json.Marshal(jsonBody)
	if err != nil {
		t.Fatalf("Failed to marshal request: %s", err)
	}
	r, err := msc2836.NewEventRelationshipRequest(bytes.NewBuffer(b))
	if err != nil {
		t.Fatalf("Failed to NewEventRelationshipRequest: %s", err)
	}
	return r
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

func postRelationships(t *testing.T, expectCode int, accessToken string, req *msc2836.EventRelationshipRequest) *msc2836.EventRelationshipResponse {
	t.Helper()
	var r msc2836.EventRelationshipRequest
	r.Defaults()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %s", err)
	}
	httpReq, err := http.NewRequest(
		"POST", "http://localhost:8009/_matrix/client/unstable/event_relationships",
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
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("wrong response code, got %d want %d - body: %s", res.StatusCode, expectCode, string(body))
	}
	if res.StatusCode == 200 {
		var result msc2836.EventRelationshipResponse
		body, err := io.ReadAll(res.Body)
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
	userapi.UserInternalAPITrace
	accessTokens map[string]userapi.Device
}

func (u *testUserAPI) QueryAccessToken(ctx context.Context, req *userapi.QueryAccessTokenRequest, res *userapi.QueryAccessTokenResponse) error {
	dev, ok := u.accessTokens[req.AccessToken]
	if !ok {
		res.Err = "unknown token"
		return nil
	}
	res.Device = &dev
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
	cfg.Defaults(config.DefaultOpts{
		Generate:   true,
		Monolithic: true,
	})
	cfg.Global.ServerName = "localhost"
	cfg.MSCs.Database.ConnectionString = "file:msc2836_test.db"
	cfg.MSCs.MSCs = []string{"msc2836"}
	base := &base.BaseDendrite{
		Cfg:                    cfg,
		PublicClientAPIMux:     mux.NewRouter().PathPrefix(httputil.PublicClientPathPrefix).Subrouter(),
		PublicFederationAPIMux: mux.NewRouter().PathPrefix(httputil.PublicFederationPathPrefix).Subrouter(),
	}

	err := msc2836.Enable(base, rsAPI, nil, userAPI, nil)
	if err != nil {
		t.Fatalf("failed to enable MSC2836: %s", err)
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
