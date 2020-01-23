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
	"sync"
	"time"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// Notifier will wake up sleeping requests when there is some new data.
// It does not tell requests what that data is, only the sync position which
// they can use to get at it. This is done to prevent races whereby we tell the caller
// the event, but the token has already advanced by the time they fetch it, resulting
// in missed events.
type Notifier struct {
	// A map of RoomID => Set<UserID> : Must only be accessed by the OnNewEvent goroutine
	roomIDToJoinedUsers map[string]userIDSet
	// Protects currPos and userStreams.
	streamLock *sync.Mutex
	// The latest sync position
	currPos types.PaginationToken
	// A map of user_id => UserStream which can be used to wake a given user's /sync request.
	userStreams map[string]*UserStream
	// The last time we cleaned out stale entries from the userStreams map
	lastCleanUpTime time.Time
}

// NewNotifier creates a new notifier set to the given sync position.
// In order for this to be of any use, the Notifier needs to be told all rooms and
// the joined users within each of them by calling Notifier.Load(*storage.SyncServerDatabase).
func NewNotifier(pos types.PaginationToken) *Notifier {
	return &Notifier{
		currPos:             pos,
		roomIDToJoinedUsers: make(map[string]userIDSet),
		userStreams:         make(map[string]*UserStream),
		streamLock:          &sync.Mutex{},
		lastCleanUpTime:     time.Now(),
	}
}

// OnNewEvent is called when a new event is received from the room server. Must only be
// called from a single goroutine, to avoid races between updates which could set the
// current sync position incorrectly.
// Chooses which user sync streams to update by a provided *gomatrixserverlib.Event
// (based on the users in the event's room),
// a roomID directly, or a list of user IDs, prioritised by parameter ordering.
// posUpdate contains the latest position(s) for one or more types of events.
// If a position in posUpdate is 0, it means no updates are available of that type.
// Typically a consumer supplies a posUpdate with the latest sync position for the
// event type it handles, leaving other fields as 0.
func (n *Notifier) OnNewEvent(
	ev *gomatrixserverlib.Event, roomID string, userIDs []string,
	posUpdate types.PaginationToken,
) {
	// update the current position then notify relevant /sync streams.
	// This needs to be done PRIOR to waking up users as they will read this value.
	n.streamLock.Lock()
	defer n.streamLock.Unlock()
	latestPos := n.currPos.WithUpdates(posUpdate)
	n.currPos = latestPos

	n.removeEmptyUserStreams()

	if ev != nil {
		// Map this event's room_id to a list of joined users, and wake them up.
		usersToNotify := n.joinedUsers(ev.RoomID())
		// If this is an invite, also add in the invitee to this list.
		if ev.Type() == "m.room.member" && ev.StateKey() != nil {
			targetUserID := *ev.StateKey()
			membership, err := ev.Membership()
			if err != nil {
				log.WithError(err).WithField("event_id", ev.EventID()).Errorf(
					"Notifier.OnNewEvent: Failed to unmarshal member event",
				)
			} else {
				// Keep the joined user map up-to-date
				switch membership {
				case gomatrixserverlib.Invite:
					usersToNotify = append(usersToNotify, targetUserID)
				case gomatrixserverlib.Join:
					// Manually append the new user's ID so they get notified
					// along all members in the room
					usersToNotify = append(usersToNotify, targetUserID)
					n.addJoinedUser(ev.RoomID(), targetUserID)
				case gomatrixserverlib.Leave:
					fallthrough
				case gomatrixserverlib.Ban:
					n.removeJoinedUser(ev.RoomID(), targetUserID)
				}
			}
		}

		n.wakeupUsers(usersToNotify, latestPos)
	} else if roomID != "" {
		n.wakeupUsers(n.joinedUsers(roomID), latestPos)
	} else if len(userIDs) > 0 {
		n.wakeupUsers(userIDs, latestPos)
	} else {
		log.WithFields(log.Fields{
			"posUpdate": posUpdate.String,
		}).Warn("Notifier.OnNewEvent called but caller supplied no user to wake up")
	}
}

// GetListener returns a UserStreamListener that can be used to wait for
// updates for a user. Must be closed.
// notify for anything before sincePos
func (n *Notifier) GetListener(req syncRequest) UserStreamListener {
	// Do what synapse does: https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/notifier.py#L298
	// - Bucket request into a lookup map keyed off a list of joined room IDs and separately a user ID
	// - Incoming events wake requests for a matching room ID
	// - Incoming events wake requests for a matching user ID (needed for invites)

	// TODO: v1 /events 'peeking' has an 'explicit room ID' which is also tracked,
	//       but given we don't do /events, let's pretend it doesn't exist.

	n.streamLock.Lock()
	defer n.streamLock.Unlock()

	n.removeEmptyUserStreams()

	return n.fetchUserStream(req.device.UserID, true).GetListener(req.ctx)
}

// Load the membership states required to notify users correctly.
func (n *Notifier) Load(ctx context.Context, db storage.Database) error {
	roomToUsers, err := db.AllJoinedUsersInRooms(ctx)
	if err != nil {
		return err
	}
	n.setUsersJoinedToRooms(roomToUsers)
	return nil
}

// CurrentPosition returns the current sync position
func (n *Notifier) CurrentPosition() types.PaginationToken {
	n.streamLock.Lock()
	defer n.streamLock.Unlock()

	return n.currPos
}

// setUsersJoinedToRooms marks the given users as 'joined' to the given rooms, such that new events from
// these rooms will wake the given users /sync requests. This should be called prior to ANY calls to
// OnNewEvent (eg on startup) to prevent racing.
func (n *Notifier) setUsersJoinedToRooms(roomIDToUserIDs map[string][]string) {
	// This is just the bulk form of addJoinedUser
	for roomID, userIDs := range roomIDToUserIDs {
		if _, ok := n.roomIDToJoinedUsers[roomID]; !ok {
			n.roomIDToJoinedUsers[roomID] = make(userIDSet)
		}
		for _, userID := range userIDs {
			n.roomIDToJoinedUsers[roomID].add(userID)
		}
	}
}

func (n *Notifier) wakeupUsers(userIDs []string, newPos types.PaginationToken) {
	for _, userID := range userIDs {
		stream := n.fetchUserStream(userID, false)
		if stream != nil {
			stream.Broadcast(newPos) // wake up all goroutines Wait()ing on this stream
		}
	}
}

// fetchUserStream retrieves a stream unique to the given user. If makeIfNotExists is true,
// a stream will be made for this user if one doesn't exist and it will be returned. This
// function does not wait for data to be available on the stream.
// NB: Callers should have locked the mutex before calling this function.
func (n *Notifier) fetchUserStream(userID string, makeIfNotExists bool) *UserStream {
	stream, ok := n.userStreams[userID]
	if !ok && makeIfNotExists {
		// TODO: Unbounded growth of streams (1 per user)
		stream = NewUserStream(userID, n.currPos)
		n.userStreams[userID] = stream
	}
	return stream
}

// Not thread-safe: must be called on the OnNewEvent goroutine only
func (n *Notifier) addJoinedUser(roomID, userID string) {
	if _, ok := n.roomIDToJoinedUsers[roomID]; !ok {
		n.roomIDToJoinedUsers[roomID] = make(userIDSet)
	}
	n.roomIDToJoinedUsers[roomID].add(userID)
}

// Not thread-safe: must be called on the OnNewEvent goroutine only
func (n *Notifier) removeJoinedUser(roomID, userID string) {
	if _, ok := n.roomIDToJoinedUsers[roomID]; !ok {
		n.roomIDToJoinedUsers[roomID] = make(userIDSet)
	}
	n.roomIDToJoinedUsers[roomID].remove(userID)
}

// Not thread-safe: must be called on the OnNewEvent goroutine only
func (n *Notifier) joinedUsers(roomID string) (userIDs []string) {
	if _, ok := n.roomIDToJoinedUsers[roomID]; !ok {
		return
	}
	return n.roomIDToJoinedUsers[roomID].values()
}

// removeEmptyUserStreams iterates through the user stream map and removes any
// that have been empty for a certain amount of time. This is a crude way of
// ensuring that the userStreams map doesn't grow forver.
// This should be called when the notifier gets called for whatever reason,
// the function itself is responsible for ensuring it doesn't iterate too
// often.
// NB: Callers should have locked the mutex before calling this function.
func (n *Notifier) removeEmptyUserStreams() {
	// Only clean up  now and again
	now := time.Now()
	if n.lastCleanUpTime.Add(time.Minute).After(now) {
		return
	}
	n.lastCleanUpTime = now

	deleteBefore := now.Add(-5 * time.Minute)
	for key, value := range n.userStreams {
		if value.TimeOfLastNonEmpty().Before(deleteBefore) {
			delete(n.userStreams, key)
		}
	}
}

// A string set, mainly existing for improving clarity of structs in this file.
type userIDSet map[string]bool

func (s userIDSet) add(str string) {
	s[str] = true
}

func (s userIDSet) remove(str string) {
	delete(s, str)
}

func (s userIDSet) values() (vals []string) {
	for str := range s {
		vals = append(vals, str)
	}
	return
}
