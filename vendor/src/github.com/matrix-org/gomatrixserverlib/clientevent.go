/* Copyright 2017 Vector Creations Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gomatrixserverlib

// EventFormat specifies the format of a client event
type EventFormat int

const (
	// FormatAll will include all client event keys
	FormatAll EventFormat = iota
	// FormatSync will include only the event keys required by the /sync API. Notably, this
	// means the 'room_id' will be missing from the events.
	FormatSync
)

// ClientEvent is an event which is fit for consumption by clients, in accordance with the specification.
type ClientEvent struct {
	Content        rawJSON   `json:"content"`
	EventID        string    `json:"event_id"`
	OriginServerTS Timestamp `json:"origin_server_ts"`
	// RoomID is omitted on /sync responses
	RoomID   string  `json:"room_id,omitempty"`
	Sender   string  `json:"sender"`
	StateKey *string `json:"state_key,omitempty"`
	Type     string  `json:"type"`
	Unsigned rawJSON `json:"unsigned,omitempty"`
}

// ToClientEvents converts server events to client events.
func ToClientEvents(serverEvs []Event, format EventFormat) []ClientEvent {
	evs := make([]ClientEvent, len(serverEvs))
	for i, se := range serverEvs {
		evs[i] = ToClientEvent(se, format)
	}
	return evs
}

// ToClientEvent converts a single server event to a client event.
func ToClientEvent(se Event, format EventFormat) ClientEvent {
	ce := ClientEvent{
		Content:        rawJSON(se.Content()),
		Sender:         se.Sender(),
		Type:           se.Type(),
		StateKey:       se.StateKey(),
		Unsigned:       rawJSON(se.Unsigned()),
		OriginServerTS: se.OriginServerTS(),
		EventID:        se.EventID(),
	}
	if format == FormatAll {
		ce.RoomID = se.RoomID()
	}
	return ce
}
