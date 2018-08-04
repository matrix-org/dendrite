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

package api

import "github.com/matrix-org/gomatrixserverlib"

// TODO: Remove this package after, typingserver/api is updated to contain a gomatrixserverlib.Event

// OutputTypingEvent is an entry in typing server output kafka log.
type OutputTypingEvent struct {
	// The Event for the typing edu event.
	Event gomatrixserverlib.Event `json:"event"`
}
