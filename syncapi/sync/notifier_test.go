// Copyright 2017 Vector Creations Ltd
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

package sync

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

var (
	randomMessageEvent  gomatrixserverlib.Event
	aliceInviteBobEvent gomatrixserverlib.Event
	bobLeaveEvent       gomatrixserverlib.Event
	syncPositionVeryOld types.SyncPosition
	syncPositionBefore  types.SyncPosition
	syncPositionAfter   types.SyncPosition
	syncPositionAfter2  types.SyncPosition
)

var (
	roomID = "!test:localhost"
	alice  = "@alice:localhost"
	bob    = "@bob:localhost"
)

func init() {
	baseSyncPos := types.SyncPosition{
		PDUPosition:    0,
		TypingPosition: 0,
	}

	syncPositionVeryOld = baseSyncPos
	syncPositionVeryOld.PDUPosition = 5

	syncPositionBefore = baseSyncPos
	syncPositionBefore.PDUPosition = 11

	syncPositionAfter = baseSyncPos
	syncPositionAfter.PDUPosition = 12

	syncPositionAfter2 = baseSyncPos
	syncPositionAfter2.PDUPosition = 13

	var err error
	randomMessageEvent, err = gomatrixserverlib.NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.message",
		"content": {
			"body": "Hello World",
			"msgtype": "m.text"
		},
		"sender": "@noone:localhost",
		"room_id": "`+roomID+`",
		"origin_server_ts": 12345,
		"event_id": "$randomMessageEvent:localhost"
	}`), false)
	if err != nil {
		panic(err)
	}
	aliceInviteBobEvent, err = gomatrixserverlib.NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.member",
		"state_key": "`+bob+`",
		"content": {
			"membership": "invite"
		},
		"sender": "`+alice+`",
		"room_id": "`+roomID+`",
		"origin_server_ts": 12345,
		"event_id": "$aliceInviteBobEvent:localhost"
	}`), false)
	if err != nil {
		panic(err)
	}
	bobLeaveEvent, err = gomatrixserverlib.NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.member",
		"state_key": "`+bob+`",
		"content": {
			"membership": "leave"
		},
		"sender": "`+bob+`",
		"room_id": "`+roomID+`",
		"origin_server_ts": 12345,
		"event_id": "$bobLeaveEvent:localhost"
	}`), false)
	if err != nil {
		panic(err)
	}
}

// Test that the current position is returned if a request is already behind.
func TestImmediateNotification(t *testing.T) {
	n := NewNotifier(syncPositionBefore)
	pos, err := waitForEvents(n, newTestSyncRequest(alice, syncPositionVeryOld))
	if err != nil {
		t.Fatalf("TestImmediateNotification error: %s", err)
	}
	if pos != syncPositionBefore {
		t.Fatalf("TestImmediateNotification want %d, got %d", syncPositionBefore, pos)
	}
}

// Test that new events to a joined room unblocks the request.
func TestNewEventAndJoinedToRoom(t *testing.T) {
	n := NewNotifier(syncPositionBefore)
	n.setUsersJoinedToRooms(map[string][]string{
		roomID: {alice, bob},
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest(bob, syncPositionBefore))
		if err != nil {
			t.Errorf("TestNewEventAndJoinedToRoom error: %s", err)
		}
		if pos != syncPositionAfter {
			t.Errorf("TestNewEventAndJoinedToRoom want %d, got %d", syncPositionAfter, pos)
		}
		wg.Done()
	}()

	stream := n.fetchUserStream(bob, true)
	waitForBlocking(stream, 1)

	n.OnNewEvent(&randomMessageEvent, "", nil, syncPositionAfter)

	wg.Wait()
}

// Test that an invite unblocks the request
func TestNewInviteEventForUser(t *testing.T) {
	n := NewNotifier(syncPositionBefore)
	n.setUsersJoinedToRooms(map[string][]string{
		roomID: {alice, bob},
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest(bob, syncPositionBefore))
		if err != nil {
			t.Errorf("TestNewInviteEventForUser error: %s", err)
		}
		if pos != syncPositionAfter {
			t.Errorf("TestNewInviteEventForUser want %d, got %d", syncPositionAfter, pos)
		}
		wg.Done()
	}()

	stream := n.fetchUserStream(bob, true)
	waitForBlocking(stream, 1)

	n.OnNewEvent(&aliceInviteBobEvent, "", nil, syncPositionAfter)

	wg.Wait()
}

// Test that all blocked requests get woken up on a new event.
func TestMultipleRequestWakeup(t *testing.T) {
	n := NewNotifier(syncPositionBefore)
	n.setUsersJoinedToRooms(map[string][]string{
		roomID: {alice, bob},
	})

	var wg sync.WaitGroup
	wg.Add(3)
	poll := func() {
		pos, err := waitForEvents(n, newTestSyncRequest(bob, syncPositionBefore))
		if err != nil {
			t.Errorf("TestMultipleRequestWakeup error: %s", err)
		}
		if pos != syncPositionAfter {
			t.Errorf("TestMultipleRequestWakeup want %d, got %d", syncPositionAfter, pos)
		}
		wg.Done()
	}
	go poll()
	go poll()
	go poll()

	stream := n.fetchUserStream(bob, true)
	waitForBlocking(stream, 3)

	n.OnNewEvent(&randomMessageEvent, "", nil, syncPositionAfter)

	wg.Wait()

	numWaiting := stream.NumWaiting()
	if numWaiting != 0 {
		t.Errorf("TestMultipleRequestWakeup NumWaiting() want 0, got %d", numWaiting)
	}
}

// Test that you stop getting woken up when you leave a room.
func TestNewEventAndWasPreviouslyJoinedToRoom(t *testing.T) {
	// listen as bob. Make bob leave room. Make alice send event to room.
	// Make sure alice gets woken up only and not bob as well.
	n := NewNotifier(syncPositionBefore)
	n.setUsersJoinedToRooms(map[string][]string{
		roomID: {alice, bob},
	})

	var leaveWG sync.WaitGroup

	// Make bob leave the room
	leaveWG.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest(bob, syncPositionBefore))
		if err != nil {
			t.Errorf("TestNewEventAndWasPreviouslyJoinedToRoom error: %s", err)
		}
		if pos != syncPositionAfter {
			t.Errorf("TestNewEventAndWasPreviouslyJoinedToRoom want %d, got %d", syncPositionAfter, pos)
		}
		leaveWG.Done()
	}()
	bobStream := n.fetchUserStream(bob, true)
	waitForBlocking(bobStream, 1)
	n.OnNewEvent(&bobLeaveEvent, "", nil, syncPositionAfter)
	leaveWG.Wait()

	// send an event into the room. Make sure alice gets it. Bob should not.
	var aliceWG sync.WaitGroup
	aliceStream := n.fetchUserStream(alice, true)
	aliceWG.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest(alice, syncPositionAfter))
		if err != nil {
			t.Errorf("TestNewEventAndWasPreviouslyJoinedToRoom error: %s", err)
		}
		if pos != syncPositionAfter2 {
			t.Errorf("TestNewEventAndWasPreviouslyJoinedToRoom want %d, got %d", syncPositionAfter2, pos)
		}
		aliceWG.Done()
	}()

	go func() {
		// this should timeout with an error (but the main goroutine won't wait for the timeout explicitly)
		_, err := waitForEvents(n, newTestSyncRequest(bob, syncPositionAfter))
		if err == nil {
			t.Errorf("TestNewEventAndWasPreviouslyJoinedToRoom expect error but got nil")
		}
	}()

	waitForBlocking(aliceStream, 1)
	waitForBlocking(bobStream, 1)

	n.OnNewEvent(&randomMessageEvent, "", nil, syncPositionAfter2)
	aliceWG.Wait()

	// it's possible that at this point alice has been informed and bob is about to be informed, so wait
	// for a fraction of a second to account for this race
	time.Sleep(1 * time.Millisecond)
}

func waitForEvents(n *Notifier, req syncRequest) (types.SyncPosition, error) {
	listener := n.GetListener(req)
	defer listener.Close()

	select {
	case <-time.After(5 * time.Second):
		return types.SyncPosition{}, fmt.Errorf(
			"waitForEvents timed out waiting for %s (pos=%d)", req.device.UserID, req.since,
		)
	case <-listener.GetNotifyChannel(*req.since):
		p := listener.GetStreamPosition()
		return p, nil
	}
}

// Wait until something is Wait()ing on the user stream.
func waitForBlocking(s *UserStream, numBlocking uint) {
	for numBlocking != s.NumWaiting() {
		// This is horrible but I don't want to add a signalling mechanism JUST for testing.
		time.Sleep(1 * time.Microsecond)
	}
}

func newTestSyncRequest(userID string, since types.SyncPosition) syncRequest {
	return syncRequest{
		device:        authtypes.Device{UserID: userID},
		timeout:       1 * time.Minute,
		since:         &since,
		wantFullState: false,
		limit:         defaultTimelineLimit,
		log:           util.GetLogger(context.TODO()),
		ctx:           context.TODO(),
	}
}
