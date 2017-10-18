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

	"github.com/matrix-org/dendrite/syncapi/types"
)

// UserStream represents a communication mechanism between the /sync request goroutine
// and the underlying sync server goroutines. Goroutines can Wait() for a stream position and
// goroutines can Broadcast(streamPosition) to other goroutines.
type UserStream struct {
	UserID string
	// The waiting channels....... TODO
	waitingChannels []chan<- types.StreamPosition
	// The lock that protects pos
	lock sync.Mutex
	// The position to broadcast to callers of Wait().
	pos types.StreamPosition
	// The time when waitingChannels was last non-empty
	timeOfLastChannel time.Time
}

// NewUserStream creates a new user stream
func NewUserStream(userID string, currPos types.StreamPosition) *UserStream {
	return &UserStream{
		UserID:            userID,
		timeOfLastChannel: time.Now(),
		pos:               currPos,
	}
}

// Wait returns a channel that produces a single stream position when
// a new event *may* be available to return to the client.
func (s *UserStream) Wait(ctx context.Context, waitAtPos types.StreamPosition) <-chan types.StreamPosition {
	posChannel := make(chan types.StreamPosition, 1)

	s.lock.Lock()
	defer s.lock.Unlock()

	// Before we start blocking, we need to make sure that we didn't race with a call
	// to Broadcast() between calling Wait() and actually sleeping. We check the last
	// broadcast pos to see if it is newer than the pos we are meant to wait at. If it
	// is newer, something has Broadcast to this stream more recently so return immediately.
	if s.pos > waitAtPos {
		posChannel <- s.pos
		close(posChannel)
		return posChannel
	}

	s.waitingChannels = append(s.waitingChannels, posChannel)

	// We spawn off a goroutine that waits for the request to finish and removes the
	// channel from waitingChannels
	go func() {
		<-ctx.Done()

		s.lock.Lock()
		defer s.lock.Unlock()

		// Icky but efficient way of filtering out the given channel
		for idx, ch := range s.waitingChannels {
			if posChannel == ch {
				lastIdx := len(s.waitingChannels) - 1
				s.waitingChannels[idx] = s.waitingChannels[lastIdx]
				s.waitingChannels[lastIdx] = nil // Ensure that the channel gets GCed
				s.waitingChannels = s.waitingChannels[:lastIdx]

				if len(s.waitingChannels) == 0 {
					s.timeOfLastChannel = time.Now()
				}

				break
			}
		}
	}()

	return posChannel
}

// Broadcast a new stream position for this user.
func (s *UserStream) Broadcast(pos types.StreamPosition) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.waitingChannels) != 0 {
		s.timeOfLastChannel = time.Now()
	}

	s.pos = pos

	for _, c := range s.waitingChannels {
		c <- pos
		close(c)
	}

	s.waitingChannels = nil
}

// NumWaiting returns the number of goroutines waiting for Wait() to return. Used for metrics and testing.
func (s *UserStream) NumWaiting() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return len(s.waitingChannels)
}

// TimeOfLastNonEmpty returns the last time that the number of waiting channels
// was non-empty, may be time.Now() if number of waiting channels is currently
// non-empty.
func (s *UserStream) TimeOfLastNonEmpty() time.Time {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.waitingChannels) > 0 {
		return time.Now()
	}
	return s.timeOfLastChannel
}
