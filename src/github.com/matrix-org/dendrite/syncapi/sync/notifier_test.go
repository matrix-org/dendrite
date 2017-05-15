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

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

var randomMessageEvent gomatrixserverlib.Event

func init() {
	var err error
	randomMessageEvent, err = gomatrixserverlib.NewEventFromTrustedJSON([]byte(`{
		"type": "m.room.message",
		"content": {
			"body": "Hello World",
			"msgtype": "m.text"
		},
		"sender": "@noone:localhost",
		"room_id": "!test:localhost",
		"origin_server_ts": 12345,
		"event_id": "$something:localhost"
	}`), false)
	if err != nil {
		panic(err)
	}
}

// Test that the current position is returned if a request is already behind.
func TestImmediateNotification(t *testing.T) {
	currPos := types.StreamPosition(11)
	n := NewNotifier(currPos)
	pos, err := waitForEvents(n, newTestSyncRequest("@alice:localhost", types.StreamPosition(9)))
	if err != nil {
		t.Fatalf("TestImmediateNotification error: %s", err)
	}
	if pos != currPos {
		t.Fatalf("TestImmediateNotification want %d, got %d", currPos, pos)
	}
}

// Test that new events to a joined room unblocks the request.
func TestNewEventAndJoinedToRoom(t *testing.T) {
	currPos := types.StreamPosition(11)
	newPos := types.StreamPosition(12)
	n := NewNotifier(currPos)
	n.usersJoinedToRooms(map[string][]string{
		"!test:localhost": []string{"@alice:localhost", "@bob:localhost"},
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		pos, err := waitForEvents(n, newTestSyncRequest("@bob:localhost", types.StreamPosition(11)))
		if err != nil {
			t.Errorf("TestNewEventAndJoinedToRoom error: %s", err)
		}
		if pos != newPos {
			t.Errorf("TestNewEventAndJoinedToRoom want %d, got %d", newPos, pos)
		}
		wg.Done()
	}()

	stream := n.fetchUserStream("@bob:localhost", true)
	waitForBlocking(stream, 1)

	n.OnNewEvent(&randomMessageEvent, types.StreamPosition(12))

	wg.Wait()
}

// Test that new events to a not joined room does not unblock the request.
func TestNewEventAndNotJoinedToRoom(t *testing.T) {

}

// Test that an invite unblocks the request
func TestNewInviteEventForUser(t *testing.T) {

}

// Test that all blocked requests get woken up on a new event.
func TestMultipleRequestWakeup(t *testing.T) {

}

// Test that you stop getting unblocked when you leave a room.
func TestNewEventAndWasPreviouslyJoinedToRoom(t *testing.T) {

}

// same as Notifier.WaitForEvents but with a timeout.
func waitForEvents(n *Notifier, req syncRequest) (types.StreamPosition, error) {
	done := make(chan types.StreamPosition, 1)
	go func() {
		newPos := n.WaitForEvents(req)
		done <- newPos
		close(done)
	}()
	select {
	case <-time.After(5 * time.Second):
		return types.StreamPosition(0), fmt.Errorf(
			"waitForEvents timed out waiting for %s (pos=%d)", req.userID, req.since,
		)
	case p := <-done:
		return p, nil
	}
}

// Wait until something is Wait()ing on the user stream.
func waitForBlocking(s *UserStream, numBlocking int) {
	for numBlocking != s.NumWaiting() {
		// This is horrible but I don't want to add a signalling mechanism JUST for testing.
		time.Sleep(1 * time.Microsecond)
	}
}

func newTestSyncRequest(userID string, since types.StreamPosition) syncRequest {
	return syncRequest{
		userID:        userID,
		timeout:       1 * time.Minute,
		since:         since,
		wantFullState: false,
		limit:         defaultTimelineLimit,
		log:           util.GetLogger(context.TODO()),
	}
}
