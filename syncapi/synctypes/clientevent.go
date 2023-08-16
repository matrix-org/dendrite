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
	"fmt"

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
		validRoomID, err := spec.NewRoomID(se.RoomID())
		if err != nil {
			continue
		}
		userID, err := userIDForSender(*validRoomID, se.SenderID())
		if err == nil && userID != nil {
			sender = *userID
		}

		sk := se.StateKey()
		if sk != nil && *sk != "" {
			skUserID, err := userIDForSender(*validRoomID, spec.SenderID(*sk))
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
	validRoomID, err := spec.NewRoomID(event.RoomID())
	if err != nil {
		return ClientEvent{}
	}
	userID, err := userIDQuery(*validRoomID, event.SenderID())
	if err == nil && userID != nil {
		sender = *userID
	}

	sk := event.StateKey()
	if sk != nil && *sk != "" {
		skUserID, err := userIDQuery(*validRoomID, spec.SenderID(*event.StateKey()))
		if err == nil && skUserID != nil {
			skString := skUserID.String()
			sk = &skString
		}
	}
	return ToClientEvent(event, FormatAll, sender, sk)
}

// If provided state key is a user ID (state keys beginning with @ are reserved for this purpose)
// fetch it's associated sender ID and use that instead. Otherwise returns the same state key back.
//
// This function either returns the state key that should be used, or a JSON error response that should be returned to the user.
// A boolean is also returned, which, if true, means the err is a result of either:
//   - State key begins with @, but does not contain a valid user ID.
//   - State key contains a valid user ID, but this user ID does not have a sender ID.
//
// TODO: it's currently unclear how translation logic should behave in the above two cases - two options for each case:
//   - Starts with @ but invalid user ID:
//     -- Reject request (e.g. as 404), but people may have state keys, that, against the spec,
//     begin with @ and dont contain a valid user ID, which they would be unable to interact with.
//     -- Silently ignore, and let them use the invalid user ID without any translation attempt. This will probably work
//     but could cause issues down the line with user ID grammar.
//   - Valid user ID, but does not have a sender ID
//     -- Reject reuquest (e.g. as 404), but people may wish to set a state event with a key containing
//     a user ID for someone who has not yet joined the room - this prevents that.
//     -- Silently ignore and don't translate - could cause issues where a state event is set with a user ID, then that user joins
//     and now querying that same state key returns different state event (as the user now has a pseudo ID)
func FromClientStateKey(roomID spec.RoomID, stateKey string, senderIDQuery spec.SenderIDForUser) (*string, bool, error) {
	if len(stateKey) >= 1 && stateKey[0] == '@' {
		parsedStateKey, err := spec.NewUserID(stateKey, true)
		if err != nil {
			// If invalid user ID, then there is no associated state event.
			return nil, true, fmt.Errorf("Provided state key begins with @ but is not a valid user ID: %s", err.Error())
		}
		senderID, err := senderIDQuery(roomID, *parsedStateKey)
		if err != nil {
			return nil, false, fmt.Errorf("Failed to query sender ID: %s", err.Error())
		}
		if senderID == nil {
			// If no sender ID, then there is no associated state event.
			return nil, true, fmt.Errorf("No associated sender ID found.")
		}
		newStateKey := string(*senderID)
		return &newStateKey, false, nil
	} else {
		return &stateKey, false, nil
	}
}
