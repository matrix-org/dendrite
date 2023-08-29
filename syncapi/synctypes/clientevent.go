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
	"encoding/json"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"
)

// PrevEventRef represents a reference to a previous event in a state event upgrade
type PrevEventRef struct {
	PrevContent   json.RawMessage `json:"prev_content"`
	ReplacesState string          `json:"replaces_state"`
	PrevSenderID  string          `json:"prev_sender"`
}

type ClientEventFormat int

const (
	// FormatAll will include all client event keys
	FormatAll ClientEventFormat = iota
	// FormatSync will include only the event keys required by the /sync API. Notably, this
	// means the 'room_id' will be missing from the events.
	FormatSync
	// FormatSyncFederation will include all event keys normally included in federated events.
	// This allows clients to request federated formatted events via the /sync API.
	FormatSyncFederation
)

// ClientFederationEvent extends a ClientEvent to contain the additional fields present in a
// federation event. Used when the client requests `event_format` of type `federation`.
type ClientFederationEvent struct {
	Depth      int64        `json:"depth,omitempty"`
	PrevEvents []string     `json:"prev_events,omitempty"`
	AuthEvents []string     `json:"auth_events,omitempty"`
	Signatures spec.RawJSON `json:"signatures,omitempty"`
	Hashes     spec.RawJSON `json:"hashes,omitempty"`
}

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

	// Federation Event Fields. Only sent to clients when `event_format` == `federation`.
	ClientFederationEvent
}

// ToClientEvents converts server events to client events.
func ToClientEvents(serverEvs []gomatrixserverlib.PDU, format ClientEventFormat, userIDForSender spec.UserIDForSender) []ClientEvent {
	evs := make([]ClientEvent, 0, len(serverEvs))
	for _, se := range serverEvs {
		if se == nil {
			continue // TODO: shouldn't happen?
		}
		if format == FormatSyncFederation {
			evs = append(evs, ToClientEvent(se, format, string(se.SenderID()), se.StateKey(), spec.RawJSON(se.Unsigned())))
			continue
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

		unsigned := se.Unsigned()
		var prev PrevEventRef
		if err := json.Unmarshal(se.Unsigned(), &prev); err == nil && prev.PrevSenderID != "" {
			prevUserID, err := userIDForSender(*validRoomID, spec.SenderID(prev.PrevSenderID))
			if err == nil && userID != nil {
				prev.PrevSenderID = prevUserID.String()
			} else {
				logrus.Warnf("Failed to find userID for prev_sender in ClientEvent")
				// NOTE: Not much can be done here, so leave the previous value in place.
			}
			unsigned, err = json.Marshal(prev)
			if err != nil {
				logrus.Errorf("Failed to marshal unsigned content for ClientEvent: %s", err.Error())
				continue
			}
		}
		evs = append(evs, ToClientEvent(se, format, sender.String(), sk, spec.RawJSON(unsigned)))
	}
	return evs
}

// ToClientEvent converts a single server event to a client event.
func ToClientEvent(se gomatrixserverlib.PDU, format ClientEventFormat, sender string, stateKey *string, unsigned spec.RawJSON) ClientEvent {
	ce := ClientEvent{
		Content:        spec.RawJSON(se.Content()),
		Sender:         sender,
		Type:           se.Type(),
		StateKey:       stateKey,
		Unsigned:       unsigned,
		OriginServerTS: se.OriginServerTS(),
		EventID:        se.EventID(),
		Redacts:        se.Redacts(),
	}

	switch format {
	case FormatAll:
		ce.RoomID = se.RoomID()
	case FormatSync:
	case FormatSyncFederation:
		ce.RoomID = se.RoomID()
		ce.AuthEvents = se.AuthEventIDs()
		ce.PrevEvents = se.PrevEventIDs()
		ce.Depth = se.Depth()
		// TODO: Set Signatures & Hashes fields
	}

	if format != FormatSyncFederation {
		if se.Version() == gomatrixserverlib.RoomVersionPseudoIDs {
			ce.SenderKey = se.SenderID()
		}
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
	return ToClientEvent(event, FormatAll, sender.String(), sk, event.Unsigned())
}

// If provided state key is a user ID (state keys beginning with @ are reserved for this purpose)
// fetch it's associated sender ID and use that instead. Otherwise returns the same state key back.
//
// # This function either returns the state key that should be used, or an error
//
// TODO: handle failure cases better (e.g. no sender ID)
func FromClientStateKey(roomID spec.RoomID, stateKey string, senderIDQuery spec.SenderIDForUser) (*string, error) {
	if len(stateKey) >= 1 && stateKey[0] == '@' {
		parsedStateKey, err := spec.NewUserID(stateKey, true)
		if err != nil {
			// If invalid user ID, then there is no associated state event.
			return nil, fmt.Errorf("Provided state key begins with @ but is not a valid user ID: %s", err.Error())
		}
		senderID, err := senderIDQuery(roomID, *parsedStateKey)
		if err != nil {
			return nil, fmt.Errorf("Failed to query sender ID: %s", err.Error())
		}
		if senderID == nil {
			// If no sender ID, then there is no associated state event.
			return nil, fmt.Errorf("No associated sender ID found.")
		}
		newStateKey := string(*senderID)
		return &newStateKey, nil
	} else {
		return &stateKey, nil
	}
}
