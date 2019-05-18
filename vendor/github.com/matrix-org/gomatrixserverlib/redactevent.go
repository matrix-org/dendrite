/* Copyright 2016-2017 Vector Creations Ltd
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

import (
	"encoding/json"
)

// RawJSON is a reimplementation of json.RawMessage that supports being used as a value type
//
// For example:
//
//  jsonBytes, _ := json.Marshal(struct{
//		RawMessage json.RawMessage
//		RawJSON RawJSON
//	}{
//		json.RawMessage(`"Hello"`),
//		RawJSON(`"World"`),
//	})
//
// Results in:
//
//  {"RawMessage":"IkhlbGxvIg==","RawJSON":"World"}
//
// See https://play.golang.org/p/FzhKIJP8-I for a full example.
type RawJSON []byte

// MarshalJSON implements the json.Marshaller interface using a value receiver.
// This means that RawJSON used as an embedded value will still encode correctly.
func (r RawJSON) MarshalJSON() ([]byte, error) {
	return []byte(r), nil
}

// UnmarshalJSON implements the json.Unmarshaller interface using a pointer receiver.
func (r *RawJSON) UnmarshalJSON(data []byte) error {
	*r = RawJSON(data)
	return nil
}

// redactEvent strips the user controlled fields from an event, but leaves the
// fields necessary for authenticating the event.
func redactEvent(eventJSON []byte) ([]byte, error) {

	// createContent keeps the fields needed in a m.room.create event.
	// Create events need to keep the creator.
	// (In an ideal world they would keep the m.federate flag see matrix-org/synapse#1831)
	type createContent struct {
		Creator RawJSON `json:"creator,omitempty"`
	}

	// joinRulesContent keeps the fields needed in a m.room.join_rules event.
	// Join rules events need to keep the join_rule key.
	type joinRulesContent struct {
		JoinRule RawJSON `json:"join_rule,omitempty"`
	}

	// powerLevelContent keeps the fields needed in a m.room.power_levels event.
	// Power level events need to keep all the levels.
	type powerLevelContent struct {
		Users         RawJSON `json:"users,omitempty"`
		UsersDefault  RawJSON `json:"users_default,omitempty"`
		Events        RawJSON `json:"events,omitempty"`
		EventsDefault RawJSON `json:"events_default,omitempty"`
		StateDefault  RawJSON `json:"state_default,omitempty"`
		Ban           RawJSON `json:"ban,omitempty"`
		Kick          RawJSON `json:"kick,omitempty"`
		Redact        RawJSON `json:"redact,omitempty"`
	}

	// memberContent keeps the fields needed in a m.room.member event.
	// Member events keep the membership.
	// (In an ideal world they would keep the third_party_invite see matrix-org/synapse#1831)
	type memberContent struct {
		Membership RawJSON `json:"membership,omitempty"`
	}

	// aliasesContent keeps the fields needed in a m.room.aliases event.
	// TODO: Alias events probably don't need to keep the aliases key, but we need to match synapse here.
	type aliasesContent struct {
		Aliases RawJSON `json:"aliases,omitempty"`
	}

	// historyVisibilityContent keeps the fields needed in a m.room.history_visibility event
	// History visibility events need to keep the history_visibility key.
	type historyVisibilityContent struct {
		HistoryVisibility RawJSON `json:"history_visibility,omitempty"`
	}

	// allContent keeps the union of all the content fields needed across all the event types.
	// All the content JSON keys we are keeping are distinct across the different event types.
	type allContent struct {
		createContent
		joinRulesContent
		powerLevelContent
		memberContent
		aliasesContent
		historyVisibilityContent
	}

	// eventFields keeps the top level keys needed by all event types.
	// (In an ideal world they would include the "redacts" key for m.room.redaction events, see matrix-org/synapse#1831)
	// See https://github.com/matrix-org/synapse/blob/v0.18.7/synapse/events/utils.py#L42-L56 for the list of fields
	type eventFields struct {
		EventID        RawJSON    `json:"event_id,omitempty"`
		Sender         RawJSON    `json:"sender,omitempty"`
		RoomID         RawJSON    `json:"room_id,omitempty"`
		Hashes         RawJSON    `json:"hashes,omitempty"`
		Signatures     RawJSON    `json:"signatures,omitempty"`
		Content        allContent `json:"content"`
		Type           string     `json:"type"`
		StateKey       RawJSON    `json:"state_key,omitempty"`
		Depth          RawJSON    `json:"depth,omitempty"`
		PrevEvents     RawJSON    `json:"prev_events,omitempty"`
		PrevState      RawJSON    `json:"prev_state,omitempty"`
		AuthEvents     RawJSON    `json:"auth_events,omitempty"`
		Origin         RawJSON    `json:"origin,omitempty"`
		OriginServerTS RawJSON    `json:"origin_server_ts,omitempty"`
		Membership     RawJSON    `json:"membership,omitempty"`
	}

	var event eventFields
	// Unmarshalling into a struct will discard any extra fields from the event.
	if err := json.Unmarshal(eventJSON, &event); err != nil {
		return nil, err
	}
	var newContent allContent
	// Copy the content fields that we should keep for the event type.
	// By default we copy nothing leaving the content object empty.
	switch event.Type {
	case MRoomCreate:
		newContent.createContent = event.Content.createContent
	case MRoomMember:
		newContent.memberContent = event.Content.memberContent
	case MRoomJoinRules:
		newContent.joinRulesContent = event.Content.joinRulesContent
	case MRoomPowerLevels:
		newContent.powerLevelContent = event.Content.powerLevelContent
	case MRoomHistoryVisibility:
		newContent.historyVisibilityContent = event.Content.historyVisibilityContent
	case MRoomAliases:
		newContent.aliasesContent = event.Content.aliasesContent
	}
	// Replace the content with our new filtered content.
	// This will zero out any keys that weren't copied in the switch statement above.
	event.Content = newContent
	// Return the redacted event encoded as JSON.
	return json.Marshal(&event)
}
