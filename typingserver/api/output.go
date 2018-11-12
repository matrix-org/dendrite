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

// OutputTypingEvent is an entry in typing server output kafka log.
// This contains the event with extra fields used to create 'm.typing' event
// in clientapi & federation.
type OutputTypingEvent struct {
	// The Event for the typing edu event.
	Event TypingEvent `json:"event"`
	// Users typing in the room when the event was generated.
	TypingUsers []string `json:"typing_users"`
}

// TypingEvent represents a matrix edu event of type 'm.typing'.
type TypingEvent struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
	Typing bool   `json:"typing"`
}
