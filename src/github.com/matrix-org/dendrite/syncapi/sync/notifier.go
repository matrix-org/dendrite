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
	"github.com/matrix-org/gomatrixserverlib"
)

// Notifier will wake up sleeping requests in the request pool when there
// is some new data. It does not tell requests what that data is, only the
// stream position which they can use to get at it.
type Notifier struct {
	// The latest sync stream position: guarded by 'cond'.
	currPos types.StreamPosition
	// A condition variable to notify all waiting goroutines of a new sync stream position
	cond *sync.Cond
}

// NewNotifier creates a new notifier set to the given stream position.
func NewNotifier(pos types.StreamPosition) *Notifier {
	return &Notifier{
		pos,
		sync.NewCond(&sync.Mutex{}),
	}
}

// OnNewEvent is called when a new event is received from the room server. Must only be
// called from a single goroutine, to avoid races between updates which could set the
// current position in the stream incorrectly.
func (n *Notifier) OnNewEvent(ev *gomatrixserverlib.Event, pos types.StreamPosition) {
	// update the current position in a guard and then notify all /sync streams
	n.cond.L.Lock()
	n.currPos = pos
	n.cond.L.Unlock()

	n.cond.Broadcast() // notify ALL waiting goroutines
}

// WaitForEvents blocks until there are new events for this request.
func (n *Notifier) WaitForEvents(req syncRequest) types.StreamPosition {
	// In a guard, check if the /sync request should block, and block it until we get a new position
	n.cond.L.Lock()
	currentPos := n.currPos
	for req.since == currentPos {
		// we need to wait for a new event.
		// TODO: This waits for ANY new event, we need to only wait for events which we care about.
		n.cond.Wait() // atomically unlocks and blocks goroutine, then re-acquires lock on unblock
		currentPos = n.currPos
	}
	n.cond.L.Unlock()
	return currentPos
}
