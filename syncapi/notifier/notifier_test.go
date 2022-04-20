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

package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

var (
	randomMessageEvent  gomatrixserverlib.HeaderedEvent
	aliceInviteBobEvent gomatrixserverlib.HeaderedEvent
	bobLeaveEvent       gomatrixserverlib.HeaderedEvent
	syncPositionVeryOld = types.StreamingToken{PDUPosition: 5}
	syncPositionBefore  = types.StreamingToken{PDUPosition: 11}
	syncPositionAfter   = types.StreamingToken{PDUPosition: 12}
	//syncPositionNewEDU  = types.NewStreamToken(syncPositionAfter.PDUPosition, 1, 0, 0, nil)
	syncPositionAfter2 = types.StreamingToken{PDUPosition: 13}
)

var (
	roomID   = "!test:localhost"
	alice    = "@alice:localhost"
	aliceDev = "alicedevice"
	bob      = "@bob:localhost"
	bobDev   = "bobdev"
)

func init() {
	var err error
	err = json.Unmarshal([]byte(`{
		"_room_version": "1",
		"type": "m.room.message",
		"content": {
			"body": "Hello World",
			"msgtype": "m.text"
		},
		"sender": "@noone:localhost",
		"room_id": "`+roomID+`",
		"origin": "localhost",
		"origin_server_ts": 12345,
		"event_id": "$randomMessageEvent:localhost"
	}`), &randomMessageEvent)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal([]byte(`{
		"_room_version": "1",
		"type": "m.room.member",
		"state_key": "`+bob+`",
		"content": {
			"membership": "invite"
		},
		"sender": "`+alice+`",
		"room_id": "`+roomID+`",
		"origin": "localhost",
		"origin_server_ts": 12345,
		"event_id": "$aliceInviteBobEvent:localhost"
	}`), &aliceInviteBobEvent)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal([]byte(`{
		"_room_version": "1",
		"type": "m.room.member",
		"state_key": "`+bob+`",
		"content": {
			"membership": "leave"
		},
		"sender": "`+bob+`",
		"room_id": "`+roomID+`",
		"origin": "localhost",
		"origin_server_ts": 12345,
		"event_id": "$bobLeaveEvent:localhost"
	}`), &bobLeaveEvent)
	if err != nil {
		panic(err)
	}
}

func mustEqualPositions(t *testing.T, got, want types.StreamingToken) {
	if got.String() != want.String() {
		t.Fatalf("mustEqualPositions got %s want %s", got.String(), want.String())
	}
}

// Test that the current position is returned if a request is already behind.
func TestImmediateNotification(t *testing.T) {
	n := NewNotifier()
	n.SetCurrentPosition(syncPositionBefore)
	pos, err := waitForEvents(n, newTestSyncRequest(alice, aliceDev, syncPositionVeryOld))
	if err != nil {
		t.Fatalf("TestImmediateNotification error: %s", err)
	}
	mustEqualPositions(t, pos, syncPositionBefore)
}

// Test that new events to a joined room unblocks the request.
func TestNewEventAndJoinedToRoom(t *testing.T) {
	n := NewNotifier()
	n.SetCurrentPosition(syncPositionBefore)
	n.setUsersJoinedToRooms(map[string][]string{
		roomID: {alice, bob},
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest(bob, bobDev, syncPositionBefore))
		if err != nil {
			t.Errorf("TestNewEventAndJoinedToRoom error: %s", err)
		}
		mustEqualPositions(t, pos, syncPositionAfter)
		wg.Done()
	}()

	stream := lockedFetchUserStream(n, bob, bobDev)
	waitForBlocking(stream, 1)

	n.OnNewEvent(&randomMessageEvent, "", nil, syncPositionAfter)

	wg.Wait()
}

func TestCorrectStream(t *testing.T) {
	n := NewNotifier()
	n.SetCurrentPosition(syncPositionBefore)
	stream := lockedFetchUserStream(n, bob, bobDev)
	if stream.UserID != bob {
		t.Fatalf("expected user %q, got %q", bob, stream.UserID)
	}
	if stream.DeviceID != bobDev {
		t.Fatalf("expected device %q, got %q", bobDev, stream.DeviceID)
	}
}

func TestCorrectStreamWakeup(t *testing.T) {
	n := NewNotifier()
	n.SetCurrentPosition(syncPositionBefore)
	awoken := make(chan string)

	streamone := lockedFetchUserStream(n, alice, "one")
	streamtwo := lockedFetchUserStream(n, alice, "two")

	go func() {
		select {
		case <-streamone.ch():
			awoken <- "one"
		case <-streamtwo.ch():
			awoken <- "two"
		}
	}()

	time.Sleep(1 * time.Second)

	wake := "two"
	n._wakeupUserDevice(alice, []string{wake}, syncPositionAfter)

	if result := <-awoken; result != wake {
		t.Fatalf("expected to wake %q, got %q", wake, result)
	}
}

// Test that an invite unblocks the request
func TestNewInviteEventForUser(t *testing.T) {
	n := NewNotifier()
	n.SetCurrentPosition(syncPositionBefore)
	n.setUsersJoinedToRooms(map[string][]string{
		roomID: {alice, bob},
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest(bob, bobDev, syncPositionBefore))
		if err != nil {
			t.Errorf("TestNewInviteEventForUser error: %s", err)
		}
		mustEqualPositions(t, pos, syncPositionAfter)
		wg.Done()
	}()

	stream := lockedFetchUserStream(n, bob, bobDev)
	waitForBlocking(stream, 1)

	n.OnNewEvent(&aliceInviteBobEvent, "", nil, syncPositionAfter)

	wg.Wait()
}

// Test an EDU-only update wakes up the request.
// TODO: Fix this test, invites wake up with an incremented
// PDU position, not EDU position
/*
func TestEDUWakeup(t *testing.T) {
	n := NewNotifier(syncPositionAfter)
	n.setUsersJoinedToRooms(map[string][]string{
		roomID: {alice, bob},
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest(bob, bobDev, syncPositionAfter))
		if err != nil {
			t.Errorf("TestNewInviteEventForUser error: %v", err)
		}
		mustEqualPositions(t, pos, syncPositionNewEDU)
		wg.Done()
	}()

	stream := lockedFetchUserStream(n, bob, bobDev)
	waitForBlocking(stream, 1)

	n.OnNewEvent(&aliceInviteBobEvent, "", nil, syncPositionNewEDU)

	wg.Wait()
}
*/

// Test that all blocked requests get woken up on a new event.
func TestMultipleRequestWakeup(t *testing.T) {
	n := NewNotifier()
	n.SetCurrentPosition(syncPositionBefore)
	n.setUsersJoinedToRooms(map[string][]string{
		roomID: {alice, bob},
	})

	var wg sync.WaitGroup
	wg.Add(3)
	poll := func() {
		pos, err := waitForEvents(n, newTestSyncRequest(bob, bobDev, syncPositionBefore))
		if err != nil {
			t.Errorf("TestMultipleRequestWakeup error: %s", err)
		}
		mustEqualPositions(t, pos, syncPositionAfter)
		wg.Done()
	}
	go poll()
	go poll()
	go poll()

	stream := lockedFetchUserStream(n, bob, bobDev)
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
	n := NewNotifier()
	n.SetCurrentPosition(syncPositionBefore)
	n.setUsersJoinedToRooms(map[string][]string{
		roomID: {alice, bob},
	})

	var leaveWG sync.WaitGroup

	// Make bob leave the room
	leaveWG.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest(bob, bobDev, syncPositionBefore))
		if err != nil {
			t.Errorf("TestNewEventAndWasPreviouslyJoinedToRoom error: %s", err)
		}
		mustEqualPositions(t, pos, syncPositionAfter)
		leaveWG.Done()
	}()
	bobStream := lockedFetchUserStream(n, bob, bobDev)
	waitForBlocking(bobStream, 1)
	n.OnNewEvent(&bobLeaveEvent, "", nil, syncPositionAfter)
	leaveWG.Wait()

	// send an event into the room. Make sure alice gets it. Bob should not.
	var aliceWG sync.WaitGroup
	aliceStream := lockedFetchUserStream(n, alice, aliceDev)
	aliceWG.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest(alice, aliceDev, syncPositionAfter))
		if err != nil {
			t.Errorf("TestNewEventAndWasPreviouslyJoinedToRoom error: %s", err)
		}
		mustEqualPositions(t, pos, syncPositionAfter2)
		aliceWG.Done()
	}()

	go func() {
		// this should timeout with an error (but the main goroutine won't wait for the timeout explicitly)
		_, err := waitForEvents(n, newTestSyncRequest(bob, bobDev, syncPositionAfter))
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

func waitForEvents(n *Notifier, req types.SyncRequest) (types.StreamingToken, error) {
	listener := n.GetListener(req)
	defer listener.Close()

	select {
	case <-time.After(5 * time.Second):
		return types.StreamingToken{}, fmt.Errorf(
			"waitForEvents timed out waiting for %s (pos=%v)", req.Device.UserID, req.Since,
		)
	case <-listener.GetNotifyChannel(req.Since):
		p := listener.GetSyncPosition()
		return p, nil
	}
}

// Wait until something is Wait()ing on the user stream.
func waitForBlocking(s *UserDeviceStream, numBlocking uint) {
	for numBlocking != s.NumWaiting() {
		// This is horrible but I don't want to add a signalling mechanism JUST for testing.
		time.Sleep(1 * time.Microsecond)
	}
}

// lockedFetchUserStream invokes Notifier.fetchUserStream, respecting Notifier.streamLock.
// A new stream is made if it doesn't exist already.
func lockedFetchUserStream(n *Notifier, userID, deviceID string) *UserDeviceStream {
	n.lock.Lock()
	defer n.lock.Unlock()

	return n._fetchUserDeviceStream(userID, deviceID, true)
}

func newTestSyncRequest(userID, deviceID string, since types.StreamingToken) types.SyncRequest {
	return types.SyncRequest{
		Device: &userapi.Device{
			UserID: userID,
			ID:     deviceID,
		},
		Timeout:       1 * time.Minute,
		Since:         since,
		WantFullState: false,
		Log:           util.GetLogger(context.TODO()),
		Context:       context.TODO(),
	}
}
