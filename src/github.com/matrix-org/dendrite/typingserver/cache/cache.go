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

var defaultTypingTimeout = 10 * time.Second

// userSet is a map of user IDs to a timer, timer fires at expiry.
type userSet map[string]*time.Timer

// TypingCache maintains a list of users typing in each room.
type TypingCache struct {
	sync.RWMutex
	data map[string]userSet
}

// NewTypingCache returns a new TypingCache initialized for use.
func NewTypingCache() *TypingCache {
	return &TypingCache{data: make(map[string]userSet)}
}

// GetTypingUsers returns the list of users typing in a room.
func (t *TypingCache) GetTypingUsers(roomID string) (users []string) {
	t.RLock()
	usersMap, ok := t.data[roomID]
	t.RUnlock()
	if ok {
		users = make([]string, 0, len(usersMap))
		for key := range usersMap {
			users = append(users, key)
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

	if t.data[roomID] == nil {
		t.data[roomID] = make(userSet)
	}

	// Stop the timer to cancel the call to timeoutCallback
	if timer, ok := t.data[roomID][userID]; ok {
		// It may happen that at this stage timer fires but now we have a lock on t.
		// Hence the execution of timeoutCallback will happen after we unlock.
		// So we may lose a typing state, though this event is highly unlikely.
		// This can be mitigated by keeping another time.Time in the map and check against it
		// before removing. This however is not required in most practical scenario.
		timer.Stop()
	}

	t.data[roomID][userID] = expiryTimer
}

// Returns a function which is called after timeout happens.
// This removes the user.
func (t *TypingCache) timeoutCallback(userID, roomID string) func() {
	return func() {
		t.removeUser(userID, roomID)
	}
}

// removeUser with mutex lock & stop the timer.
func (t *TypingCache) removeUser(userID, roomID string) {
	t.Lock()
	defer t.Unlock()

	if timer, ok := t.data[roomID][userID]; ok {
		timer.Stop()
		delete(t.data[roomID], userID)
	}
}

func getExpireTime(expire *time.Time) time.Time {
	if expire != nil {
		return *expire
	}
	return time.Now().Add(defaultTypingTimeout)
}
