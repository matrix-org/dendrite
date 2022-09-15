package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
)

const userPassword = "this_is_a_long_password"

type user struct {
	userID    string
	localpart string
	client    *gomatrix.Client
}

// runTests performs the following operations:
// - register alice and bob with branch name muxed into the localpart
// - create a DM room for the 2 users and exchange messages
// - create/join a public #global room and exchange messages
func runTests(baseURL, branchName string) error {
	// register 2 users
	users := []user{
		{
			localpart: "alice" + branchName,
		},
		{
			localpart: "bob" + branchName,
		},
	}
	for i, u := range users {
		client, err := gomatrix.NewClient(baseURL, "", "")
		if err != nil {
			return err
		}
		resp, err := client.RegisterDummy(&gomatrix.ReqRegister{
			Username: strings.ToLower(u.localpart),
			Password: userPassword,
		})
		if err != nil {
			return fmt.Errorf("failed to register %s: %s", u.localpart, err)
		}
		client, err = gomatrix.NewClient(baseURL, resp.UserID, resp.AccessToken)
		if err != nil {
			return err
		}
		users[i].client = client
		users[i].userID = resp.UserID
	}

	// create DM room, join it and exchange messages
	createRoomResp, err := users[0].client.CreateRoom(&gomatrix.ReqCreateRoom{
		Preset:   "trusted_private_chat",
		Invite:   []string{users[1].userID},
		IsDirect: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create DM room: %s", err)
	}
	dmRoomID := createRoomResp.RoomID
	if _, err = users[1].client.JoinRoom(dmRoomID, "", nil); err != nil {
		return fmt.Errorf("failed to join DM room: %s", err)
	}
	msgs := []struct {
		client *gomatrix.Client
		text   string
	}{
		{
			client: users[0].client, text: "1: " + branchName,
		},
		{
			client: users[1].client, text: "2: " + branchName,
		},
		{
			client: users[0].client, text: "3: " + branchName,
		},
		{
			client: users[1].client, text: "4: " + branchName,
		},
	}
	wantEventIDs := make(map[string]struct{}, 8)
	for _, msg := range msgs {
		var resp *gomatrix.RespSendEvent
		resp, err = msg.client.SendText(dmRoomID, msg.text)
		if err != nil {
			return fmt.Errorf("failed to send text in dm room: %s", err)
		}
		wantEventIDs[resp.EventID] = struct{}{}
	}

	// attempt to create/join the shared public room
	publicRoomID := ""
	createRoomResp, err = users[0].client.CreateRoom(&gomatrix.ReqCreateRoom{
		RoomAliasName: "global",
		Preset:        "public_chat",
	})
	if err != nil { // this is okay and expected if the room already exists and the aliases clash
		// try to join it
		_, domain, err2 := gomatrixserverlib.SplitID('@', users[0].userID)
		if err2 != nil {
			return fmt.Errorf("failed to split user ID: %s, %s", users[0].userID, err2)
		}
		joinRoomResp, err2 := users[0].client.JoinRoom(fmt.Sprintf("#global:%s", domain), "", nil)
		if err2 != nil {
			return fmt.Errorf("alice failed to join public room: %s", err2)
		}
		publicRoomID = joinRoomResp.RoomID
	} else {
		publicRoomID = createRoomResp.RoomID
	}
	if _, err = users[1].client.JoinRoom(publicRoomID, "", nil); err != nil {
		return fmt.Errorf("bob failed to join public room: %s", err)
	}
	// send messages
	for _, msg := range msgs {
		resp, err := msg.client.SendText(publicRoomID, "public "+msg.text)
		if err != nil {
			return fmt.Errorf("failed to send text in public room: %s", err)
		}
		wantEventIDs[resp.EventID] = struct{}{}
	}

	// Sync until we have all expected messages
	doneCh := make(chan struct{})
	go func() {
		syncClient := users[0].client
		since := ""
		for len(wantEventIDs) > 0 {
			select {
			case <-doneCh:
				return
			default:
			}
			syncResp, err := syncClient.SyncRequest(1000, since, "1", false, "")
			if err != nil {
				continue
			}
			for _, room := range syncResp.Rooms.Join {
				for _, ev := range room.Timeline.Events {
					if ev.Type != "m.room.message" {
						continue
					}
					delete(wantEventIDs, ev.ID)
				}
			}
			since = syncResp.NextBatch
		}
		close(doneCh)
	}()

	select {
	case <-time.After(time.Second * 10):
		close(doneCh)
		return fmt.Errorf("failed to receive all expected messages: %+v", wantEventIDs)
	case <-doneCh:
	}

	log.Printf("OK! rooms(public=%s, dm=%s) users(%s, %s)\n", publicRoomID, dmRoomID, users[0].userID, users[1].userID)
	return nil
}

// verifyTestsRan checks that the HS has the right rooms/messages
func verifyTestsRan(baseURL string, branchNames []string) error {
	log.Println("Verifying tests....")
	// check we can login as all users
	var resp *gomatrix.RespLogin
	for _, branchName := range branchNames {
		client, err := gomatrix.NewClient(baseURL, "", "")
		if err != nil {
			return err
		}
		userLocalparts := []string{
			"alice" + branchName,
			"bob" + branchName,
		}
		for _, userLocalpart := range userLocalparts {
			resp, err = client.Login(&gomatrix.ReqLogin{
				Type:     "m.login.password",
				User:     strings.ToLower(userLocalpart),
				Password: userPassword,
			})
			if err != nil {
				return fmt.Errorf("failed to login as %s: %s", userLocalpart, err)
			}
			if resp.AccessToken == "" {
				return fmt.Errorf("failed to login, bad response: %+v", resp)
			}
		}
	}
	log.Println("    accounts exist: OK")
	client, err := gomatrix.NewClient(baseURL, resp.UserID, resp.AccessToken)
	if err != nil {
		return err
	}
	_, domain, err := gomatrixserverlib.SplitID('@', client.UserID)
	if err != nil {
		return err
	}
	u := client.BuildURL("directory", "room", fmt.Sprintf("#global:%s", domain))
	r := struct {
		RoomID string `json:"room_id"`
	}{}
	err = client.MakeRequest("GET", u, nil, &r)
	if err != nil {
		return fmt.Errorf("failed to /directory: %s", err)
	}
	if r.RoomID == "" {
		return fmt.Errorf("/directory lookup returned no room ID")
	}
	log.Println("    public room exists: OK")

	history, err := client.Messages(r.RoomID, client.Store.LoadNextBatch(client.UserID), "", 'b', 100)
	if err != nil {
		return fmt.Errorf("failed to get /messages: %s", err)
	}
	// we expect 4 messages per version
	msgCount := 0
	for _, ev := range history.Chunk {
		if ev.Type == "m.room.message" {
			msgCount += 1
		}
	}
	wantMsgCount := len(branchNames) * 4
	if msgCount != wantMsgCount {
		return fmt.Errorf("got %d messages in global room, want %d", msgCount, wantMsgCount)
	}
	log.Println("    messages exist: OK")
	return nil
}
