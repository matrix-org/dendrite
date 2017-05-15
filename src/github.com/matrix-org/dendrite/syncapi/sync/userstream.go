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
	"sync"

	"github.com/matrix-org/dendrite/syncapi/types"
)

// UserStream represents a communication mechanism between the /sync request goroutine
// and the underlying sync server goroutines. Goroutines can Wait() for a stream position and
// goroutines can Broadcast(streamPosition) to other goroutines.
type UserStream struct {
	UserID string
	// Because this is a Cond, we can notify all waiting goroutines so this works
	// across devices for the same user. Protects pos.
	cond *sync.Cond
	// The position to broadcast to callers of Wait().
	pos types.StreamPosition
	// The number of goroutines blocked on Wait() - used for testing and metrics
	numWaiting int
}

// NewUserStream creates a new user stream
func NewUserStream(userID string) *UserStream {
	return &UserStream{
		UserID: userID,
		cond:   sync.NewCond(&sync.Mutex{}),
	}
}

// Wait blocks until there is a new stream position for this user, which is then returned.
func (s *UserStream) Wait() (pos types.StreamPosition) {
	s.cond.L.Lock()
	s.numWaiting++
	s.cond.Wait()
	pos = s.pos
	s.numWaiting--
	s.cond.L.Unlock()
	return
}

// Broadcast a new stream position for this user.
func (s *UserStream) Broadcast(pos types.StreamPosition) {
	s.cond.L.Lock()
	s.pos = pos
	s.cond.L.Unlock()
	s.cond.Broadcast()
}

// NumWaiting returns the number of goroutines waiting for Wait() to return. Used for metrics and testing.
func (s *UserStream) NumWaiting() int {
	s.cond.L.Lock()
	defer s.cond.L.Unlock()
	return s.numWaiting
}
