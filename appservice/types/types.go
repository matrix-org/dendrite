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

package types

import (
	"sync"

	"github.com/matrix-org/dendrite/setup/config"
)

const (
	// AppServiceDeviceID is the AS dummy device ID
	AppServiceDeviceID = "AS_Device"
)

// ApplicationServiceWorkerState is a type that couples an application service,
// a lockable condition as well as some other state variables, allowing the
// roomserver to notify appservice workers when there are events ready to send
// externally to application services.
type ApplicationServiceWorkerState struct {
	AppService config.ApplicationService
	Cond       *sync.Cond
	// Lastest incremental ID from appservice_events table that is ready to be sent to application service
	latestId int
	// Backoff exponent (2^x secs). Max 6, aka 64s.
	Backoff int
}

// NotifyNewEvents wakes up all waiting goroutines, notifying that events remain
// in the event queue for this application service worker.
func (a *ApplicationServiceWorkerState) NotifyNewEvents(id int) {
	a.Cond.L.Lock()
	a.latestId = id
	a.Cond.Broadcast()
	a.Cond.L.Unlock()
}

// WaitForNewEvents causes the calling goroutine to wait on the worker state's
// condition for a broadcast or similar wakeup, if there are no events ready.
func (a *ApplicationServiceWorkerState) WaitForNewEvents(id int) {
	a.Cond.L.Lock()
	if a.latestId <= id {
		a.Cond.Wait()
	}
	a.Cond.L.Unlock()
}
