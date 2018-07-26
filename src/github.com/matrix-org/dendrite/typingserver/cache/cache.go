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

// userSet is a map of user IDs to their time of expiry.
type userSet map[string]time.Time

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
	if time.Until(expireTime) > 0 {
		t.addUser(userID, roomID, expireTime)
		t.removeUserAfterTime(userID, roomID, expireTime)
	}
}

// addUser with mutex lock.
func (t *TypingCache) addUser(userID, roomID string, expireTime time.Time) {
	t.Lock()
	defer t.Unlock()

	if t.data[roomID] == nil {
		t.data[roomID] = make(userSet)
	}

	t.data[roomID][userID] = expireTime
}

// Creates a go routine which removes the user after expireTime has elapsed,
// only if the expiration is not updated to a later time in cache.
func (t *TypingCache) removeUserAfterTime(userID, roomID string, expireTime time.Time) {
	go func() {
		time.Sleep(time.Until(expireTime))
		t.removeUserIfExpired(userID, roomID)
	}()
}

// removeUserIfExpired with mutex lock.
func (t *TypingCache) removeUserIfExpired(userID, roomID string) {
	t.Lock()
	defer t.Unlock()

	if time.Until(t.data[roomID][userID]) <= 0 {
		delete(t.data[roomID], userID)
	}
}

func getExpireTime(expire *time.Time) time.Time {
	if expire != nil {
		return *expire
	}
	return time.Now().Add(defaultTypingTimeout)
}
