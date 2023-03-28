package clientapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
)

func TestTyping(t *testing.T) {
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()
		natsInstance := jetstream.NATSInstance{}

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		// Needed to create accounts
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
		// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]string{
			alice: "",
		}
		createAccessTokens(t, accessTokens, userAPI, ctx, routers)

		// Create the room
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatal(err)
		}

		testCases := []struct {
			name          string
			typingForUser string
			roomID        string
			requestBody   io.Reader
			wantOK        bool
		}{
			{
				name:          "can not set typing for different user",
				typingForUser: "@notourself:test",
				roomID:        room.ID,
				requestBody:   strings.NewReader(""),
			},
			{
				name:          "invalid request body",
				typingForUser: alice.ID,
				roomID:        room.ID,
				requestBody:   strings.NewReader(""),
			},
			{
				name:          "non-existent room",
				typingForUser: alice.ID,
				roomID:        "!doesnotexist:test",
			},
			{
				name:          "invalid room ID",
				typingForUser: alice.ID,
				roomID:        "@notaroomid:test",
			},
			{
				name:          "allowed to set own typing status",
				typingForUser: alice.ID,
				roomID:        room.ID,
				requestBody:   strings.NewReader(`{"typing":true}`),
				wantOK:        true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPut, "/_matrix/client/v3/rooms/"+tc.roomID+"/typing/"+tc.typingForUser, tc.requestBody)
				req.Header.Set("Authorization", "Bearer "+accessTokens[alice])
				routers.Client.ServeHTTP(rec, req)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}
			})
		}
	})
}

func TestMembership(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	room := test.NewRoom(t, alice)
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		cfg.ClientAPI.RateLimiting.Enabled = false
		defer close()
		natsInstance := jetstream.NATSInstance{}

		routers := httputil.NewRouters()
		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, &natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		// Needed to create accounts
		userAPI := userapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, rsAPI, nil)
		rsAPI.SetUserAPI(userAPI)
		// We mostly need the rsAPI/userAPI for this test, so nil for other APIs etc.
		AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, rsAPI, nil, nil, nil, userAPI, nil, nil, caching.DisableMetrics)

		// Create the users in the userapi and login
		accessTokens := map[*test.User]string{
			alice: "",
			bob:   "",
		}
		createAccessTokens(t, accessTokens, userAPI, ctx, routers)

		// Create the room
		if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
			t.Fatal(err)
		}

		invalidBodyRequest := func(roomID, membershipType string) *http.Request {
			return httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", roomID, membershipType), strings.NewReader(""))
		}

		missingUserIDRequest := func(roomID, membershipType string) *http.Request {
			return httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", roomID, membershipType), strings.NewReader("{}"))
		}

		testCases := []struct {
			name    string
			roomID  string
			request *http.Request
			wantOK  bool
			asUser  *test.User
		}{
			{
				name:    "ban - invalid request body",
				request: invalidBodyRequest(room.ID, "ban"),
			},
			{
				name:    "kick - invalid request body",
				request: invalidBodyRequest(room.ID, "kick"),
			},
			{
				name:    "unban - invalid request body",
				request: invalidBodyRequest(room.ID, "unban"),
			},
			{
				name:    "invite - invalid request body",
				request: invalidBodyRequest(room.ID, "invite"),
			},
			{
				name:    "ban - missing user_id body",
				request: missingUserIDRequest(room.ID, "ban"),
			},
			{
				name:    "kick - missing user_id body",
				request: missingUserIDRequest(room.ID, "kick"),
			},
			{
				name:    "unban - missing user_id body",
				request: missingUserIDRequest(room.ID, "unban"),
			},
			{
				name:    "invite - missing user_id body",
				request: missingUserIDRequest(room.ID, "invite"),
			},
			{
				name:    "Bob forgets invalid room",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", "!doesnotexist", "forget"), strings.NewReader("")),
				asUser:  bob,
			},
			{
				name:    "Alice can not ban Bob in non-existent room", // fails because "not joined"
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", "!doesnotexist:test", "ban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
			},
			{
				name:    "Alice can not kick Bob in non-existent room", // fails because "not joined"
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", "!doesnotexist:test", "kick"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
			},
			// the following must run in sequence, as they build up on each other
			{
				name:    "Alice invites Bob",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "invite"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Bob accepts invite",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "join"), strings.NewReader("")),
				wantOK:  true,
				asUser:  bob,
			},
			{
				name:    "Alice verifies that Bob is joined", // returns an error if no membership event can be found
				request: httptest.NewRequest(http.MethodGet, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s/m.room.member/%s", room.ID, "state", bob.ID), strings.NewReader("")),
				wantOK:  true,
			},
			{
				name:    "Bob forgets the room but is still a member",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "forget"), strings.NewReader("")),
				wantOK:  false, // user is still in the room
				asUser:  bob,
			},
			{
				name:    "Bob can not kick Alice",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "kick"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, alice.ID))),
				wantOK:  false, // powerlevel too low
				asUser:  bob,
			},
			{
				name:    "Bob can not ban Alice",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "ban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, alice.ID))),
				wantOK:  false, // powerlevel too low
				asUser:  bob,
			},
			{
				name:    "Alice can kick Bob",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "kick"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Alice can ban Bob",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "ban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Alice can not kick Bob again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "kick"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  false, // can not kick banned/left user
			},
			{
				name:    "Bob can not unban himself", // mostly because of not being a member of the room
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "unban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				asUser:  bob,
			},
			{
				name:    "Alice can not invite Bob again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "invite"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  false, // user still banned
			},
			{
				name:    "Alice can unban Bob",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "unban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Alice can not unban Bob again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "unban"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  false,
			},
			{
				name:    "Alice can invite Bob again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "invite"), strings.NewReader(fmt.Sprintf(`{"user_id":"%s"}`, bob.ID))),
				wantOK:  true,
			},
			{
				name:    "Bob can reject the invite by leaving",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "leave"), strings.NewReader("")),
				wantOK:  true,
				asUser:  bob,
			},
			{
				name:    "Bob can forget the room",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "forget"), strings.NewReader("")),
				wantOK:  true,
				asUser:  bob,
			},
			{
				name:    "Bob can forget the room again",
				request: httptest.NewRequest(http.MethodPost, fmt.Sprintf("/_matrix/client/v3/rooms/%s/%s", room.ID, "forget"), strings.NewReader("")),
				wantOK:  true,
				asUser:  bob,
			},
			// END must run in sequence
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.asUser == nil {
					tc.asUser = alice
				}
				rec := httptest.NewRecorder()
				tc.request.Header.Set("Authorization", "Bearer "+accessTokens[tc.asUser])
				routers.Client.ServeHTTP(rec, tc.request)
				if tc.wantOK && rec.Code != http.StatusOK {
					t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
				}
				if !tc.wantOK && rec.Code == http.StatusOK {
					t.Fatalf("expected request to fail, but didn't: %s", rec.Body.String())
				}
				t.Logf("%s", rec.Body.String())
			})
		}
	})
}
