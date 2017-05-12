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
	"encoding/json"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/events"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// Notifier will wake up sleeping requests when there is some new data.
// It does not tell requests what that data is, only the stream position which
// they can use to get at it. This is done to prevent races whereby we tell the caller
// the event, but the token has already advanced by the time they fetch it, resulting
// in missed events.
type Notifier struct {
	// The latest sync stream position: guarded by 'currPosMutex' which is RW to allow
	// for concurrent reads on /sync requests
	currPos      types.StreamPosition
	currPosMutex *sync.RWMutex
	// A map of RoomID => Set<UserID> : Map access is guarded by roomIDToJoinedUsersMutex.
	roomIDToJoinedUsers      map[string]set
	roomIDToJoinedUsersMutex *sync.Mutex
	// A map of user_id => Cond which can be used to wake a given user's /sync request.
	// Because this is a Cond, we can notify all waiting goroutines so this works
	// across devices. Map access is guarded by userIDCondsMutex.
	userIDConds      map[string]*sync.Cond
	userIDCondsMutex *sync.Mutex
}

// NewNotifier creates a new notifier set to the given stream position.
// In order for this to be of any use, the Notifier needs to be told all rooms and
// the joined users within each of them by calling Notifier.LoadFromDatabase().
func NewNotifier(pos types.StreamPosition) *Notifier {
	return &Notifier{
		currPos:                  pos,
		currPosMutex:             &sync.RWMutex{},
		roomIDToJoinedUsers:      make(map[string]set),
		roomIDToJoinedUsersMutex: &sync.Mutex{},
		userIDConds:              make(map[string]*sync.Cond),
		userIDCondsMutex:         &sync.Mutex{},
	}
}

// OnNewEvent is called when a new event is received from the room server. Must only be
// called from a single goroutine, to avoid races between updates which could set the
// current position in the stream incorrectly.
func (n *Notifier) OnNewEvent(ev *gomatrixserverlib.Event, pos types.StreamPosition) {
	// update the current position in a guard and then notify relevant /sync streams.
	// This needs to be done PRIOR to waking up users as they will read this value.
	n.currPosMutex.Lock()
	n.currPos = pos
	n.currPosMutex.Unlock()

	// Map this event's room_id to a list of joined users, and wake them up.
	userIDs := n.joinedUsers(ev.RoomID())
	// If this is an invite, also add in the invitee to this list.
	if ev.Type() == "m.room.member" && ev.StateKey() != nil {
		userID := *ev.StateKey()
		var memberContent events.MemberContent
		if err := json.Unmarshal(ev.Content(), &memberContent); err != nil {
			log.WithError(err).WithField("event_id", ev.EventID()).Errorf(
				"Notifier.OnNewEvent: Failed to unmarshal member event",
			)
		} else {
			// Keep the joined user map up-to-date
			switch memberContent.Membership {
			case "invite":
				userIDs = append(userIDs, userID)
			case "join":
				n.userJoined(ev.RoomID(), userID)
			case "leave":
				fallthrough
			case "ban":
				n.userLeft(ev.RoomID(), userID)
			}
		}
	}

	for _, userID := range userIDs {
		n.wakeupUser(userID)
	}
}

// WaitForEvents blocks until there are new events for this request. If forceBlock is true, the request
// will be forcibly waited until a new event wakes it up. This is typically only useful for testing
// blocking code.
func (n *Notifier) WaitForEvents(req syncRequest, forceBlock bool) types.StreamPosition {
	// Do what synapse does: https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/notifier.py#L298
	// - Bucket request into a lookup map keyed off a list of joined room IDs and separately a user ID
	// - Incoming events wake requests for a matching room ID
	// - Incoming events wake requests for a matching user ID (needed for invites)

	// TODO: v1 /events 'peeking' has an 'explicit room ID' which is also tracked,
	//       but given we don't do /events, let's pretend it doesn't exist.

	var hasBlocked bool
	for {
		// In a guard, check if the /sync request should block, and block it until we get woken up
		n.currPosMutex.RLock()
		currentPos := n.currPos
		n.currPosMutex.RUnlock()

		// TODO: We increment the stream position for any event, so it's possible that we return immediately
		//       with a pos which contains no new events for this user. We should probably re-wait for events
		//       automatically in this case.
		if req.since != currentPos {
			if !forceBlock || (forceBlock && hasBlocked) {
				return currentPos
			}
		}

		// wait to be woken up, and then re-check the stream position
		req.log.WithField("user_id", req.userID).Info("Waiting for event")
		n.blockUser(req.userID)
		hasBlocked = true
	}
}

// Load the membership states required to notify users correctly.
func (n *Notifier) Load(db *storage.SyncServerDatabase) error {
	roomToUsers, err := db.AllJoinedUsersInRooms()
	if err != nil {
		return err
	}
	n.usersJoinedToRooms(roomToUsers)
	return nil
}

// usersJoinedToRooms marks the given users as 'joined' to the given rooms, such that new events from
// these rooms will wake the given users /sync requests. This should be called prior to ANY calls to
// OnNewEvent (eg on startup) to prevent racing.
func (n *Notifier) usersJoinedToRooms(roomIDToUserIDs map[string][]string) {
	// This is just the bulk form of userJoined where we only lock once.
	n.roomIDToJoinedUsersMutex.Lock()
	defer n.roomIDToJoinedUsersMutex.Unlock()
	for roomID, userIDs := range roomIDToUserIDs {
		if _, ok := n.roomIDToJoinedUsers[roomID]; !ok {
			n.roomIDToJoinedUsers[roomID] = make(set)
		}
		for _, userID := range userIDs {
			n.roomIDToJoinedUsers[roomID].add(userID)
		}
	}
}

func (n *Notifier) wakeupUser(userID string) {
	cond := n.fetchUserCond(userID, false)
	if cond == nil {
		return
	}
	cond.Broadcast() // wakeup all goroutines Wait()ing on this Cond
}

func (n *Notifier) blockUser(userID string) {
	cond := n.fetchUserCond(userID, true)
	cond.L.Lock()
	cond.Wait()
	cond.L.Unlock()
}

// fetchUserCond retrieves a Cond unique to the given user. If makeIfNotExists is true,
// a Cond will be made for this user if one doesn't exist and it will be returned. This
// function does not lock the Cond.
func (n *Notifier) fetchUserCond(userID string, makeIfNotExists bool) *sync.Cond {
	// There is a bit of a locking dance here, we want to lock the mutex protecting the map
	// but NOT the Cond that we may be returning/creating.
	n.userIDCondsMutex.Lock()
	defer n.userIDCondsMutex.Unlock()
	cond, ok := n.userIDConds[userID]
	if !ok {
		// TODO: Unbounded growth of locks (1 per user)
		cond = sync.NewCond(&sync.Mutex{})
		n.userIDConds[userID] = cond
	}
	return cond
}

func (n *Notifier) userJoined(roomID, userID string) {
	n.roomIDToJoinedUsersMutex.Lock()
	defer n.roomIDToJoinedUsersMutex.Unlock()
	if _, ok := n.roomIDToJoinedUsers[roomID]; !ok {
		n.roomIDToJoinedUsers[roomID] = make(set)
	}
	n.roomIDToJoinedUsers[roomID].add(userID)
}

func (n *Notifier) userLeft(roomID, userID string) {
	n.roomIDToJoinedUsersMutex.Lock()
	defer n.roomIDToJoinedUsersMutex.Unlock()
	if _, ok := n.roomIDToJoinedUsers[roomID]; !ok {
		n.roomIDToJoinedUsers[roomID] = make(set)
	}
	n.roomIDToJoinedUsers[roomID].remove(userID)
}

func (n *Notifier) joinedUsers(roomID string) (userIDs []string) {
	n.roomIDToJoinedUsersMutex.Lock()
	defer n.roomIDToJoinedUsersMutex.Unlock()
	if _, ok := n.roomIDToJoinedUsers[roomID]; !ok {
		return
	}
	return n.roomIDToJoinedUsers[roomID].values()
}

// A string set, mainly existing for improving clarity of structs in this file.
type set map[string]bool

func (s set) add(str string) {
	s[str] = true
}

func (s set) remove(str string) {
	delete(s, str)
}

func (s set) has(str string) bool {
	_, ok := s[str]
	return ok
}

func (s set) values() (vals []string) {
	for str := range s {
		vals = append(vals, str)
	}
	return
}
