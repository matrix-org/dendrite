// Copyright 2017 Vector Creations Ltd
// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package caching

import (
	"sync"
	"time"
)

const defaultTypingTimeout = 10 * time.Second

// userSet is a map of user IDs to a timer, timer fires at expiry.
type userSet map[string]*time.Timer

// TimeoutCallbackFn is a function called right after the removal of a user
// from the typing user list due to timeout.
// latestSyncPosition is the typing sync position after the removal.
type TimeoutCallbackFn func(userID, roomID string, latestSyncPosition int64)

type roomData struct {
	syncPosition int64
	userSet      userSet
}

// EDUCache maintains a list of users typing in each room.
type EDUCache struct {
	sync.RWMutex
	latestSyncPosition int64
	data               map[string]*roomData
	timeoutCallback    TimeoutCallbackFn
}

// Create a roomData with its sync position set to the latest sync position.
// Must only be called after locking the cache.
func (t *EDUCache) newRoomData() *roomData {
	return &roomData{
		syncPosition: t.latestSyncPosition,
		userSet:      make(userSet),
	}
}

// NewTypingCache returns a new EDUCache initialised for use.
func NewTypingCache() *EDUCache {
	return &EDUCache{data: make(map[string]*roomData)}
}

// SetTimeoutCallback sets a callback function that is called right after
// a user is removed from the typing user list due to timeout.
func (t *EDUCache) SetTimeoutCallback(fn TimeoutCallbackFn) {
	t.timeoutCallback = fn
}

// GetTypingUsers returns the list of users typing in a room.
func (t *EDUCache) GetTypingUsers(roomID string) []string {
	users, _ := t.GetTypingUsersIfUpdatedAfter(roomID, 0)
	// 0 should work above because the first position used will be 1.
	return users
}

// GetTypingUsersIfUpdatedAfter returns all users typing in this room with
// updated == true if the typing sync position of the room is after the given
// position. Otherwise, returns an empty slice with updated == false.
func (t *EDUCache) GetTypingUsersIfUpdatedAfter(
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
// Returns the latest sync position for typing after update.
func (t *EDUCache) AddTypingUser(
	userID, roomID string, expire *time.Time,
) int64 {
	expireTime := getExpireTime(expire)
	if until := time.Until(expireTime); until > 0 {
		timer := time.AfterFunc(until, func() {
			latestSyncPosition := t.RemoveUser(userID, roomID)
			if t.timeoutCallback != nil {
				t.timeoutCallback(userID, roomID, latestSyncPosition)
			}
		})
		return t.addUser(userID, roomID, timer)
	}
	return t.GetLatestSyncPosition()
}

// addUser with mutex lock & replace the previous timer.
// Returns the latest typing sync position after update.
func (t *EDUCache) addUser(
	userID, roomID string, expiryTimer *time.Timer,
) int64 {
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
		// It may happen that at this stage the timer fires, but we now have a lock on
		// it. Hence the execution of timeoutCallback will happen after we unlock. So
		// we may lose a typing state, though this is highly unlikely. This can be
		// mitigated by keeping another time.Time in the map and checking against it
		// before removing, but its occurrence is so infrequent it does not seem
		// worthwhile.
		timer.Stop()
	}

	t.data[roomID].userSet[userID] = expiryTimer

	return t.latestSyncPosition
}

// RemoveUser with mutex lock & stop the timer.
// Returns the latest sync position for typing after update.
func (t *EDUCache) RemoveUser(userID, roomID string) int64 {
	t.Lock()
	defer t.Unlock()

	roomData, ok := t.data[roomID]
	if !ok {
		return t.latestSyncPosition
	}

	timer, ok := roomData.userSet[userID]
	if !ok {
		return t.latestSyncPosition
	}

	timer.Stop()
	delete(roomData.userSet, userID)

	t.latestSyncPosition++
	t.data[roomID].syncPosition = t.latestSyncPosition

	return t.latestSyncPosition
}

func (t *EDUCache) GetLatestSyncPosition() int64 {
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
