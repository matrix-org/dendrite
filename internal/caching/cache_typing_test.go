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
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/test"
)

func TestEDUCache(t *testing.T) {
	tCache := NewTypingCache()
	if tCache == nil {
		t.Fatal("NewTypingCache failed")
	}

	t.Run("AddTypingUser", func(t *testing.T) {
		testAddTypingUser(t, tCache)
	})

	t.Run("GetTypingUsers", func(t *testing.T) {
		testGetTypingUsers(t, tCache)
	})

	t.Run("RemoveUser", func(t *testing.T) {
		testRemoveUser(t, tCache)
	})
}

func testAddTypingUser(t *testing.T, tCache *EDUCache) { // nolint: unparam
	present := time.Now()
	tests := []struct {
		userID string
		roomID string
		expire *time.Time
	}{ // Set four users typing state to room1
		{"user1", "room1", nil},
		{"user2", "room1", nil},
		{"user3", "room1", nil},
		{"user4", "room1", nil},
		//typing state with past expireTime should not take effect or removed.
		{"user1", "room2", &present},
	}

	for _, tt := range tests {
		tCache.AddTypingUser(tt.userID, tt.roomID, tt.expire)
	}
}

func testGetTypingUsers(t *testing.T, tCache *EDUCache) {
	tests := []struct {
		roomID    string
		wantUsers []string
	}{
		{"room1", []string{"user1", "user2", "user3", "user4"}},
		{"room2", []string{}},
	}

	for _, tt := range tests {
		gotUsers := tCache.GetTypingUsers(tt.roomID)
		if !test.UnsortedStringSliceEqual(gotUsers, tt.wantUsers) {
			t.Errorf("TypingCache.GetTypingUsers(%s) = %v, want %v", tt.roomID, gotUsers, tt.wantUsers)
		}
	}
}

func testRemoveUser(t *testing.T, tCache *EDUCache) {
	tests := []struct {
		roomID  string
		userIDs []string
	}{
		{"room3", []string{"user1"}},
		{"room4", []string{"user1", "user2", "user3"}},
	}

	for _, tt := range tests {
		for _, userID := range tt.userIDs {
			tCache.AddTypingUser(userID, tt.roomID, nil)
		}

		length := len(tt.userIDs)
		tCache.RemoveUser(tt.userIDs[length-1], tt.roomID)
		expLeftUsers := tt.userIDs[:length-1]
		if leftUsers := tCache.GetTypingUsers(tt.roomID); !test.UnsortedStringSliceEqual(leftUsers, expLeftUsers) {
			t.Errorf("Response after removal is unexpected. Want = %s, got = %s", leftUsers, expLeftUsers)
		}
	}
}
