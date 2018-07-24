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
	"reflect"
	"sort"
	"testing"
	"time"
)

const defaultInterval = time.Second

func TestTypingCache(t *testing.T) {
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

	t.Run("GetTypingUsersAfterTimeout", func(t *testing.T) {
		testGetTypingUsersAfterTimeout(t, tCache)
	})
}

func testAddTypingUser(t *testing.T, tCache *TypingCache) {
	timeAfterDefaultInterval := time.Now().Add(defaultInterval)
	timeAfterTwiceDefaultInterval := time.Now().Add(2 * defaultInterval)
	tests := []struct {
		userID string
		roomID string
		expire *time.Time
	}{
		{"user1", "room1", nil},
		{"user2", "room1", nil},
		{"user3", "room1", nil},
		{"user4", "room1", nil},
		{"user1", "room2", &timeAfterDefaultInterval},
		{"user1", "room2", &timeAfterTwiceDefaultInterval},
	}

	for _, tt := range tests {
		tCache.AddTypingUser(tt.userID, tt.roomID, tt.expire)
	}
}

func testGetTypingUsers(t *testing.T, tCache *TypingCache) {
	tests := []struct {
		roomID    string
		wantUsers []string
	}{
		{"room1", []string{"user1", "user2", "user3", "user4"}},
		{"room2", []string{"user1"}},
	}

	for _, tt := range tests {
		gotUsers := tCache.GetTypingUsers(tt.roomID)
		sort.Strings(gotUsers)
		sort.Strings(tt.wantUsers)
		if !reflect.DeepEqual(gotUsers, tt.wantUsers) {
			t.Errorf("TypingCache.GetTypingUsers(%s) = %v, want %v", tt.roomID, gotUsers, tt.wantUsers)
		}
	}
}

func testGetTypingUsersAfterTimeout(t *testing.T, tCache *TypingCache) {
	time.Sleep(defaultInterval)
	tests := []struct {
		roomID    string
		wantUsers []string
	}{
		{"room2", []string{"user1"}},
	}

	for _, tt := range tests {
		if gotUsers := tCache.GetTypingUsers(tt.roomID); !reflect.DeepEqual(gotUsers, tt.wantUsers) {
			t.Errorf("TypingCache.GetTypingUsers(%s) = %v, want %v", tt.roomID, gotUsers, tt.wantUsers)
		}
	}
}
