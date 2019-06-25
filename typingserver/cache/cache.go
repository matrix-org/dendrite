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

package cache

import (
	"sync"
	"time"
)

const defaultTypingTimeout = 10 * time.Second

// userSet is a map of user IDs to a timer, timer fires at expiry.
type userSet map[string]*time.Timer

type roomData struct {
	syncPosition int64
	userSet      userSet
}

// TypingCache maintains a list of users typing in each room.
type TypingCache struct {
	sync.RWMutex
	latestSyncPosition int64
	data               map[string]*roomData
}

// Create a roomData with its sync position set to the latest sync position.
// Must only be called after locking the cache.
func (t *TypingCache) newRoomData() *roomData {
	return &roomData{
		syncPosition: t.latestSyncPosition,
		userSet:      make(userSet),
	}
}

// NewTypingCache returns a new TypingCache initialised for use.
func NewTypingCache() *TypingCache {
	return &TypingCache{data: make(map[string]*roomData)}
}

// GetTypingUsers returns the list of users typing in a room.
func (t *TypingCache) GetTypingUsers(roomID string) []string {
	users, _ := t.GetTypingUsersIfUpdatedAfter(roomID, 0)
	// 0 should work above because the first position used will be 1.
	return users
}

// GetTypingUsersIfUpdatedAfter returns all users typing in this room with
// updated == true if the typing sync position of the room is after the given
// position. Otherwise, returns an empty slice with updated == false.
func (t *TypingCache) GetTypingUsersIfUpdatedAfter(
	roomID string, position int64,
) (users []string, updated bool) {
	t.RLock()
	defer t.RUnlock()

	roomData, ok := t.data[roomID]
	if ok && roomData.syncPosition > position {
		updated = true
		userSet := roomData.userSet
		users = make([]string, 0, len(userSet))
		for userID := range userSet {
			users = append(users, userID)
		}
	}

	return
}

// AddTypingUser sets an user as typing in a room.
// expire is the time when the user typing should time out.
// if expire is nil, defaultTypingTimeout is assumed.
func (t *TypingCache) AddTypingUser(userID, roomID string, expire *time.Time) {
	expireTime := getExpireTime(expire)
	if until := time.Until(expireTime); until > 0 {
		timer := time.AfterFunc(until, t.timeoutCallback(userID, roomID))
		t.addUser(userID, roomID, timer)
	}
}

// addUser with mutex lock & replace the previous timer.
func (t *TypingCache) addUser(userID, roomID string, expiryTimer *time.Timer) {
	t.Lock()
	defer t.Unlock()

	t.latestSyncPosition++

	if t.data[roomID] == nil {
		t.data[roomID] = t.newRoomData()
	} else {
		t.data[roomID].syncPosition = t.latestSyncPosition
	}

	// Stop the timer to cancel the call to timeoutCallback
	if timer, ok := t.data[roomID].userSet[userID]; ok {
		// It may happen that at this stage timer fires but now we have a lock on t.
		// Hence the execution of timeoutCallback will happen after we unlock.
		// So we may lose a typing state, though this event is highly unlikely.
		// This can be mitigated by keeping another time.Time in the map and check against it
		// before removing. This however is not required in most practical scenario.
		timer.Stop()
	}

	t.data[roomID].userSet[userID] = expiryTimer
}

// Returns a function which is called after timeout happens.
// This removes the user.
func (t *TypingCache) timeoutCallback(userID, roomID string) func() {
	return func() {
		t.RemoveUser(userID, roomID)
	}
}

// RemoveUser with mutex lock & stop the timer.
func (t *TypingCache) RemoveUser(userID, roomID string) {
	t.Lock()
	defer t.Unlock()

	roomData, ok := t.data[roomID]
	if !ok {
		return
	}

	timer, ok := roomData.userSet[userID]
	if !ok {
		return
	}

	timer.Stop()
	delete(roomData.userSet, userID)

	t.latestSyncPosition++
	t.data[roomID].syncPosition = t.latestSyncPosition
}

func (t *TypingCache) GetLatestSyncPosition() int64 {
	t.Lock()
	defer t.Unlock()
	return t.latestSyncPosition
}

func getExpireTime(expire *time.Time) time.Time {
	if expire != nil {
		return *expire
	}
	return time.Now().Add(defaultTypingTimeout)
}
