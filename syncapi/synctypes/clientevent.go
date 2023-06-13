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

package synctypes

import (
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type ClientEventFormat int

const (
	// FormatAll will include all client event keys
	FormatAll ClientEventFormat = iota
	// FormatSync will include only the event keys required by the /sync API. Notably, this
	// means the 'room_id' will be missing from the events.
	FormatSync
)

// ClientEvent is an event which is fit for consumption by clients, in accordance with the specification.
type ClientEvent struct {
	Content        spec.RawJSON   `json:"content"`
	EventID        string         `json:"event_id,omitempty"`         // EventID is omitted on receipt events
	OriginServerTS spec.Timestamp `json:"origin_server_ts,omitempty"` // OriginServerTS is omitted on receipt events
	RoomID         string         `json:"room_id,omitempty"`          // RoomID is omitted on /sync responses
	Sender         string         `json:"sender,omitempty"`           // Sender is omitted on receipt events
	SenderKey      spec.SenderID  `json:"sender_key,omitempty"`       // The SenderKey for events in pseudo ID rooms
	StateKey       *string        `json:"state_key,omitempty"`
	Type           string         `json:"type"`
	Unsigned       spec.RawJSON   `json:"unsigned,omitempty"`
	Redacts        string         `json:"redacts,omitempty"`
}

// ToClientEvents converts server events to client events.
func ToClientEvents(serverEvs []gomatrixserverlib.PDU, format ClientEventFormat, userIDForSender spec.UserIDForSender) []ClientEvent {
	evs := make([]ClientEvent, 0, len(serverEvs))
	for _, se := range serverEvs {
		if se == nil {
			continue // TODO: shouldn't happen?
		}
		sender := spec.UserID{}
		userID, err := userIDForSender(se.RoomID(), se.SenderID())
		if err == nil && userID != nil {
			sender = *userID
		}

		sk := se.StateKey()
		if sk != nil && *sk != "" {
			skUserID, err := userIDForSender(se.RoomID(), spec.SenderID(*sk))
			if err == nil && skUserID != nil {
				skString := skUserID.String()
				sk = &skString
			}
		}
		evs = append(evs, ToClientEvent(se, format, sender, sk))
	}
	return evs
}

// ToClientEvent converts a single server event to a client event.
func ToClientEvent(se gomatrixserverlib.PDU, format ClientEventFormat, sender spec.UserID, stateKey *string) ClientEvent {
	ce := ClientEvent{
		Content:        spec.RawJSON(se.Content()),
		Sender:         sender.String(),
		Type:           se.Type(),
		StateKey:       stateKey,
		Unsigned:       spec.RawJSON(se.Unsigned()),
		OriginServerTS: se.OriginServerTS(),
		EventID:        se.EventID(),
		Redacts:        se.Redacts(),
	}
	if format == FormatAll {
		ce.RoomID = se.RoomID()
	}
	if se.Version() == gomatrixserverlib.RoomVersionPseudoIDs {
		ce.SenderKey = se.SenderID()
	}
	return ce
}

// ToClientEvent converts a single server event to a client event.
// It provides default logic for event.SenderID & event.StateKey -> userID conversions.
func ToClientEventDefault(userIDQuery spec.UserIDForSender, event gomatrixserverlib.PDU) ClientEvent {
	sender := spec.UserID{}
	userID, err := userIDQuery(event.RoomID(), event.SenderID())
	if err == nil && userID != nil {
		sender = *userID
	}

	sk := event.StateKey()
	if sk != nil && *sk != "" {
		skUserID, err := userIDQuery(event.RoomID(), spec.SenderID(*event.StateKey()))
		if err == nil && skUserID != nil {
			skString := skUserID.String()
			sk = &skString
		}
	}
	return ToClientEvent(event, FormatAll, sender, sk)
}
