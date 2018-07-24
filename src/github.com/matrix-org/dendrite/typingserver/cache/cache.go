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

var (
	userExists           = struct{}{} // Value denoting user is present in a userSet
	defaultTypingTimeout = 10 * time.Second
)

// userSet is a map of user IDs
type userSet map[string]struct{}

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
		t.addUser(userID, roomID)
		t.removeUserAfterDuration(userID, roomID, until)
	}
}

func (t *TypingCache) addUser(userID, roomID string) {
	t.Lock()
	if t.data[roomID] == nil {
		t.data[roomID] = make(userSet)
	}

	t.data[roomID][userID] = userExists
	t.Unlock()
}

// Creates a go routine which removes the user after d duration has elapsed.
func (t *TypingCache) removeUserAfterDuration(userID, roomID string, d time.Duration) {
	go func() {
		time.Sleep(d)
		t.removeUser(userID, roomID)
	}()
}

func (t *TypingCache) removeUser(userID, roomID string) {
	t.Lock()
	delete(t.data[roomID], userID)
	t.Unlock()
}

func getExpireTime(expire *time.Time) time.Time {
	if expire != nil {
		return *expire
	}
	return time.Now().Add(defaultTypingTimeout)
}
