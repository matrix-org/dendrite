// Copyright 2018 New Vector Ltd
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

package types

import (
	"sync"

	"github.com/matrix-org/dendrite/common/config"
)

// ApplicationServiceWorkerState is a type that pairs and application service with a
// lockable condition, allowing the roomserver to notify appservice workers when
// there are events ready to send externally to application services.
type ApplicationServiceWorkerState struct {
	AppService  config.ApplicationService
	Cond        *sync.Cond
	EventsReady bool
}
