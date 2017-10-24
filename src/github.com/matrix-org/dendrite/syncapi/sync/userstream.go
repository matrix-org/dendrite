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
	"runtime"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/util"
)

// UserStream represents a communication mechanism between the /sync request goroutine
// and the underlying sync server goroutines.
// Goroutines can get a UserStreamListener to wait for updates, and can Broadcast()
// updates.
type UserStream struct {
	UserID string
	// The lock that protects changes to this struct
	lock sync.Mutex
	// Closed when there is an update.
	signalChannel chan struct{}
	// The last stream position that there may have been an update for the suser
	pos types.StreamPosition
	// The last time when we had some listeners waiting
	timeOfLastChannel time.Time
	// The number of listeners waiting
	numWaiting uint
}

// UserStreamListener allows a sync request to wait for updates for a user.
type UserStreamListener struct {
	userStream *UserStream

	// Whether the stream has been closed
	hasClosed bool
}

// NewUserStream creates a new user stream
func NewUserStream(userID string, currPos types.StreamPosition) *UserStream {
	return &UserStream{
		UserID:            userID,
		timeOfLastChannel: time.Now(),
		pos:               currPos,
		signalChannel:     make(chan struct{}),
	}
}

// GetListener returns UserStreamListener that a sync request can use to wait
// for new updates with.
// UserStreamListener must be closed
func (s *UserStream) GetListener(ctx context.Context) UserStreamListener {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.numWaiting++ // We decrement when UserStreamListener is closed

	listener := UserStreamListener{
		userStream: s,
	}

	// Lets be a bit paranoid here and check that Close() is being called
	runtime.SetFinalizer(&listener, func(l *UserStreamListener) {
		if !l.hasClosed {
			util.GetLogger(ctx).Warn("Didn't call Close on UserStreamListener")
			l.Close()
		}
	})

	return listener
}

// Broadcast a new stream position for this user.
func (s *UserStream) Broadcast(pos types.StreamPosition) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.pos = pos

	close(s.signalChannel)

	s.signalChannel = make(chan struct{})
}

// NumWaiting returns the number of goroutines waiting for waiting for updates.
// Used for metrics and testing.
func (s *UserStream) NumWaiting() uint {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.numWaiting
}

// TimeOfLastNonEmpty returns the last time that the number of waiting listeners
// was non-empty, may be time.Now() if number of waiting listeners is currently
// non-empty.
func (s *UserStream) TimeOfLastNonEmpty() time.Time {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.numWaiting > 0 {
		return time.Now()
	}

	return s.timeOfLastChannel
}

// GetStreamPosition returns last stream position which the UserStream was
// notified about
func (s *UserStreamListener) GetStreamPosition() types.StreamPosition {
	s.userStream.lock.Lock()
	defer s.userStream.lock.Unlock()

	return s.userStream.pos
}

// GetNotifyChannel returns a channel that is closed when there may be an
// update for the user.
// sincePos specifies from which point we want to be notified about
func (s *UserStreamListener) GetNotifyChannel(sincePos types.StreamPosition) <-chan struct{} {
	s.userStream.lock.Lock()
	defer s.userStream.lock.Unlock()

	if sincePos < s.userStream.pos {
		// If the listener is behind, i.e. missed a potential update, then we
		// want them to wake up immediately. We do this by returning a new
		// closed stream, which returns immediately when selected.
		closedChannel := make(chan struct{})
		close(closedChannel)
		return closedChannel
	}

	return s.userStream.signalChannel
}

// Close cleans up resources used
func (s *UserStreamListener) Close() {
	s.userStream.lock.Lock()
	defer s.userStream.lock.Unlock()

	if !s.hasClosed {
		s.userStream.numWaiting--
		s.userStream.timeOfLastChannel = time.Now()
	}

	s.hasClosed = true
}
