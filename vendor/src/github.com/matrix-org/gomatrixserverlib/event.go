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
	"fmt"
	"golang.org/x/crypto/ed25519"
	"time"
)

// A StateKeyTuple is the combination of an event type and an event state key.
// It is often used as a key in maps.
type StateKeyTuple struct {
	// The "type" key of a matrix event.
	EventType string
	// The "state_key" of a matrix event.
	// The empty string is a legitimate value for the "state_key" in matrix
	// so take care to initialise this field lest you accidentally request a
	// "state_key" with the go default of the empty string.
	StateKey string
}

// An EventReference is a reference to a matrix event.
type EventReference struct {
	// The event ID of the event.
	EventID string
	// The sha256 of the redacted event.
	EventSHA256 Base64String
}

// An EventBuilder is used to build a new event.
// These can be exchanged between matrix servers in the federation APIs when
// joining or leaving a room.
type EventBuilder struct {
	// The user ID of the user sending the event.
	Sender string `json:"sender"`
	// The room ID of the room this event is in.
	RoomID string `json:"room_id"`
	// The type of the event.
	Type string `json:"type"`
	// The state_key of the event if the event is a state event or nil if the event is not a state event.
	StateKey *string `json:"state_key,omitempty"`
	// The events that immediately preceeded this event in the room history.
	PrevEvents []EventReference `json:"prev_events"`
	// The events needed to authenticate this event.
	AuthEvents []EventReference `json:"auth_events"`
	// The event ID of the event being redacted if this event is a "m.room.redaction".
	Redacts string `json:"redacts,omitempty"`
	// The depth of the event, This should be one greater than the maximum depth of the previous events.
	// The create event has a depth of 1.
	Depth int64 `json:"depth"`
	// The JSON object for "content" key of the event.
	Content rawJSON `json:"content"`
	// The JSON object for the "unsigned" key
	Unsigned rawJSON `json:"unsigned,omitempty"`
}

// SetContent sets the JSON content key of the event.
func (eb *EventBuilder) SetContent(content interface{}) (err error) {
	eb.Content, err = json.Marshal(content)
	return
}

// SetUnsigned sets the JSON unsigned key of the event.
func (eb *EventBuilder) SetUnsigned(unsigned interface{}) (err error) {
	eb.Unsigned, err = json.Marshal(unsigned)
	return
}

// An Event is a matrix event.
// The event should always contain valid JSON.
// If the event content hash is invalid then the event is redacted.
// Redacted events contain only the fields covered by the event signature.
type Event struct {
	redacted  bool
	eventJSON []byte
	fields    eventFields
}

type eventFields struct {
	RoomID         string           `json:"room_id"`
	EventID        string           `json:"event_id"`
	Sender         string           `json:"sender"`
	Type           string           `json:"type"`
	StateKey       *string          `json:"state_key"`
	Content        rawJSON          `json:"content"`
	PrevEvents     []EventReference `json:"prev_events"`
	AuthEvents     []EventReference `json:"auth_events"`
	Redacts        string           `json:"redacts"`
	Depth          int64            `json:"depth"`
	Unsigned       rawJSON          `json:"unsigned"`
	OriginServerTS Timestamp        `json:"origin_server_ts"`
	Origin         ServerName       `json:"origin"`
}

var emptyEventReferenceList = []EventReference{}

// Build a new Event.
// This is used when a local event is created on this server.
// Call this after filling out the necessary fields.
// This can be called mutliple times on the same builder.
// A different event ID must be supplied each time this is called.
func (eb *EventBuilder) Build(eventID string, now time.Time, origin ServerName, keyID KeyID, privateKey ed25519.PrivateKey) (result Event, err error) {
	var event struct {
		EventBuilder
		EventID        string     `json:"event_id"`
		OriginServerTS Timestamp  `json:"origin_server_ts"`
		Origin         ServerName `json:"origin"`
		// This key is either absent or an empty list.
		// If it is absent then the pointer is nil and omitempty removes it.
		// Otherwise it points to an empty list and omitempty keeps it.
		PrevState *[]EventReference `json:"prev_state,omitempty"`
	}
	event.EventBuilder = *eb
	if event.PrevEvents == nil {
		event.PrevEvents = emptyEventReferenceList
	}
	if event.AuthEvents == nil {
		event.AuthEvents = emptyEventReferenceList
	}
	event.OriginServerTS = AsTimestamp(now)
	event.Origin = origin
	event.EventID = eventID

	if event.StateKey != nil {
		// In early versions of the matrix protocol state events
		// had a "prev_state" key that listed the state events with
		// the same type and state key that this event replaced.
		// This was later dropped from the protocol.
		// Synapse ignores the contents of the key but still expects
		// the key to be present in state events.
		event.PrevState = &emptyEventReferenceList
	}

	var eventJSON []byte

	if eventJSON, err = json.Marshal(&event); err != nil {
		return
	}

	if eventJSON, err = addContentHashesToEvent(eventJSON); err != nil {
		return
	}

	if eventJSON, err = signEvent(string(origin), keyID, privateKey, eventJSON); err != nil {
		return
	}

	if eventJSON, err = CanonicalJSON(eventJSON); err != nil {
		return
	}

	result.eventJSON = eventJSON
	if err = json.Unmarshal(eventJSON, &result.fields); err != nil {
		return
	}

	if err = result.CheckFields(); err != nil {
		return
	}

	return
}

// NewEventFromUntrustedJSON loads a new event from some JSON that may be invalid.
// This checks that the event is valid JSON.
// It also checks the content hashes to ensure the event has not been tampered with.
// This should be used when receiving new events from remote servers.
func NewEventFromUntrustedJSON(eventJSON []byte) (result Event, err error) {
	var event map[string]rawJSON
	if err = json.Unmarshal(eventJSON, &event); err != nil {
		return
	}
	// Synapse removes these keys from events in case a server accidentally added them.
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/crypto/event_signing.py#L57-L62
	delete(event, "outlier")
	delete(event, "destinations")
	delete(event, "age_ts")

	if eventJSON, err = json.Marshal(event); err != nil {
		return
	}

	if err = checkEventContentHash(eventJSON); err != nil {
		result.redacted = true
		// If the content hash doesn't match then we have to discard all non-essential fields
		// because they've been tampered with.
		if eventJSON, err = redactEvent(eventJSON); err != nil {
			return
		}
	}

	if eventJSON, err = CanonicalJSON(eventJSON); err != nil {
		return
	}

	result.eventJSON = eventJSON
	if err = json.Unmarshal(eventJSON, &result.fields); err != nil {
		return
	}

	if err = result.CheckFields(); err != nil {
		return
	}

	return
}

// NewEventFromTrustedJSON loads a new event from some JSON that must be valid.
// This will be more efficient than NewEventFromUntrustedJSON since it can skip cryptographic checks.
// This can be used when loading matrix events from a local database.
func NewEventFromTrustedJSON(eventJSON []byte, redacted bool) (result Event, err error) {
	result.redacted = redacted
	result.eventJSON = eventJSON
	err = json.Unmarshal(eventJSON, &result.fields)
	return
}

// Redacted returns whether the event is redacted.
func (e Event) Redacted() bool { return e.redacted }

// JSON returns the JSON bytes for the event.
func (e Event) JSON() []byte { return e.eventJSON }

// Redact returns a redacted copy of the event.
func (e Event) Redact() Event {
	if e.redacted {
		return e
	}
	eventJSON, err := redactEvent(e.eventJSON)
	if err != nil {
		// This is unreachable for events created with EventBuilder.Build or NewEventFromUntrustedJSON
		panic(fmt.Errorf("gomatrixserverlib: invalid event %v", err))
	}
	if eventJSON, err = CanonicalJSON(eventJSON); err != nil {
		// This is unreachable for events created with EventBuilder.Build or NewEventFromUntrustedJSON
		panic(fmt.Errorf("gomatrixserverlib: invalid event %v", err))
	}
	return Event{
		redacted:  true,
		eventJSON: eventJSON,
	}
}

// EventReference returns an EventReference for the event.
// The reference can be used to refer to this event from other events.
func (e Event) EventReference() EventReference {
	reference, err := referenceOfEvent(e.eventJSON)
	if err != nil {
		// This is unreachable for events created with EventBuilder.Build or NewEventFromUntrustedJSON
		// This can be reached if NewEventFromTrustedJSON is given JSON from an untrusted source.
		panic(fmt.Errorf("gomatrixserverlib: invalid event %v (%q)", err, string(e.eventJSON)))
	}
	return reference
}

// Sign returns a copy of the event with an additional signature.
func (e Event) Sign(signingName string, keyID KeyID, privateKey ed25519.PrivateKey) Event {
	eventJSON, err := signEvent(signingName, keyID, privateKey, e.eventJSON)
	if err != nil {
		// This is unreachable for events created with EventBuilder.Build or NewEventFromUntrustedJSON
		panic(fmt.Errorf("gomatrixserverlib: invalid event %v (%q)", err, string(e.eventJSON)))
	}
	if eventJSON, err = CanonicalJSON(eventJSON); err != nil {
		// This is unreachable for events created with EventBuilder.Build or NewEventFromUntrustedJSON
		panic(fmt.Errorf("gomatrixserverlib: invalid event %v (%q)", err, string(e.eventJSON)))
	}
	return Event{
		redacted:  e.redacted,
		eventJSON: eventJSON,
	}
}

// KeyIDs returns a list of key IDs that the named entity has signed the event with.
func (e Event) KeyIDs(signingName string) []KeyID {
	keyIDs, err := ListKeyIDs(signingName, e.eventJSON)
	if err != nil {
		// This should unreachable for events created with EventBuilder.Build or NewEventFromUntrustedJSON
		panic(fmt.Errorf("gomatrixserverlib: invalid event %v", err))
	}
	return keyIDs
}

// Verify checks a ed25519 signature
func (e Event) Verify(signingName string, keyID KeyID, publicKey ed25519.PublicKey) error {
	return verifyEventSignature(signingName, keyID, publicKey, e.eventJSON)
}

// StateKey returns the "state_key" of the event, or the nil if the event is not a state event.
func (e Event) StateKey() *string {
	return e.fields.StateKey
}

// StateKeyEquals returns true if the event is a state event and the "state_key" matches.
func (e Event) StateKeyEquals(stateKey string) bool {
	if e.fields.StateKey == nil {
		return false
	}
	return *e.fields.StateKey == stateKey
}

const (
	// The event ID, room ID, sender, event type and state key fields cannot be
	// bigger than this.
	// https://github.com/matrix-org/synapse/blob/v0.21.0/synapse/event_auth.py#L173-L182
	maxIDLength = 255
	// The entire event JSON, including signatures cannot be bigger than this.
	// https://github.com/matrix-org/synapse/blob/v0.21.0/synapse/event_auth.py#L183-184
	maxEventLength = 65536
)

// CheckFields checks that the event fields are valid.
// Returns an error if the IDs have the wrong format or too long.
// Returns an error if the total length of the event JSON is too long.
// Returns an error if the event ID doesn't match the origin of the event.
// https://matrix.org/docs/spec/client_server/r0.2.0.html#size-limits
func (e Event) CheckFields() error {
	if len(e.eventJSON) > maxEventLength {
		return fmt.Errorf(
			"gomatrixserverlib: event is too long, length %d > maximum %d",
			len(e.eventJSON), maxEventLength,
		)
	}

	if len(e.fields.Type) > maxIDLength {
		return fmt.Errorf(
			"gomatrixserverlib: event type is too long, length %d > maximum %d",
			len(e.fields.Type), maxIDLength,
		)
	}

	if e.fields.StateKey != nil && len(*e.fields.StateKey) > maxIDLength {
		return fmt.Errorf(
			"gomatrixserverlib: state key is too long, length %d > maximum %d",
			len(*e.fields.StateKey), maxIDLength,
		)
	}

	_, err := checkID(e.fields.RoomID, "room", '!')
	if err != nil {
		return err
	}

	senderDomain, err := checkID(e.fields.Sender, "user", '@')
	if err != nil {
		return err
	}

	eventDomain, err := checkID(e.fields.EventID, "event", '$')
	if err != nil {
		return err
	}

	// Synapse requires that the event ID domain has a valid signature.
	// https://github.com/matrix-org/synapse/blob/v0.21.0/synapse/event_auth.py#L66-L68
	// Synapse requires that the event origin has a valid signature.
	// https://github.com/matrix-org/synapse/blob/v0.21.0/synapse/federation/federation_base.py#L133-L136
	// Since both domains must be valid domains, and there is no good reason for them
	// to be different we might as well ensure that they are the same since it
	// makes the signature checks simpler.
	if e.fields.Origin != ServerName(eventDomain) {
		return fmt.Errorf(
			"gomatrixserverlib: event ID domain doesn't match origin: %q != %q",
			eventDomain, e.fields.Origin,
		)
	}

	if eventDomain != senderDomain {
		// For the most part all events should be sent by a user on the
		// originating server
		// However "m.room.member" events created from third-party invites
		// are allowed to have a different sender because they have the same
		// sender as the "m.room.third_party_invite" event they derived from.
		// https://github.com/matrix-org/synapse/blob/v0.21.0/synapse/event_auth.py#L58-L64
		if e.fields.Type != MRoomMember {
			return fmt.Errorf(
				"gomatrixserverlib: sender domain doesn't match origin: %q != %q",
				eventDomain, e.fields.Origin,
			)
		}
		c, err := newMemberContentFromEvent(e)
		if err != nil {
			return err
		}
		if c.Membership != invite || c.ThirdPartyInvite == nil {
			return fmt.Errorf(
				"gomatrixserverlib: sender domain doesn't match origin: %q != %q",
				eventDomain, e.fields.Origin,
			)
		}
	}

	return nil
}

func checkID(id, kind string, sigil byte) (domain string, err error) {
	domain, err = domainFromID(id)
	if err != nil {
		return
	}
	if id[0] != sigil {
		err = fmt.Errorf(
			"gomatrixserverlib: invalid %s ID, wanted first byte to be '%c' got '%c'",
			kind, sigil, id[0],
		)
		return
	}
	if len(id) > maxIDLength {
		err = fmt.Errorf(
			"gomatrixserverlib: %s ID is too long, length %d > maximum %d",
			kind, len(id), maxIDLength,
		)
		return
	}
	return
}

// Origin returns the name of the server that sent the event
func (e Event) Origin() ServerName { return e.fields.Origin }

// EventID returns the event ID of the event.
func (e Event) EventID() string {
	return e.fields.EventID
}

// Sender returns the user ID of the sender of the event.
func (e Event) Sender() string {
	return e.fields.Sender
}

// Type returns the type of the event.
func (e Event) Type() string {
	return e.fields.Type
}

// OriginServerTS returns the unix timestamp when this event was created on the origin server, with millisecond resolution.
func (e Event) OriginServerTS() Timestamp {
	return e.fields.OriginServerTS
}

// Unsigned returns the object under the 'unsigned' key of the event.
func (e Event) Unsigned() []byte {
	return []byte(e.fields.Unsigned)
}

// Content returns the content JSON of the event.
func (e Event) Content() []byte {
	return []byte(e.fields.Content)
}

// PrevEvents returns references to the direct ancestors of the event.
func (e Event) PrevEvents() []EventReference {
	return e.fields.PrevEvents
}

// AuthEvents returns references to the events needed to auth the event.
func (e Event) AuthEvents() []EventReference {
	return e.fields.AuthEvents
}

// Redacts returns the event ID of the event this event redacts.
func (e Event) Redacts() string {
	return e.fields.Redacts
}

// RoomID returns the room ID of the room the event is in.
func (e Event) RoomID() string {
	return e.fields.RoomID
}

// Depth returns the depth of the event.
func (e Event) Depth() int64 {
	return e.fields.Depth
}

// UnmarshalJSON implements json.Unmarshaller assuming the Event is from an untrusted source.
// This will cause more checks than might be necessary but is probably better to be safe than sorry.
func (e *Event) UnmarshalJSON(data []byte) (err error) {
	*e, err = NewEventFromUntrustedJSON(data)
	return
}

// MarshalJSON implements json.Marshaller
func (e Event) MarshalJSON() ([]byte, error) {
	if e.eventJSON == nil {
		return nil, fmt.Errorf("gomatrixserverlib: cannot serialise uninitialised Event")
	}
	return e.eventJSON, nil
}

// UnmarshalJSON implements json.Unmarshaller
func (er *EventReference) UnmarshalJSON(data []byte) error {
	var tuple []rawJSON
	if err := json.Unmarshal(data, &tuple); err != nil {
		return err
	}
	if len(tuple) != 2 {
		return fmt.Errorf("gomatrixserverlib: invalid event reference, invalid length: %d != 2", len(tuple))
	}
	if err := json.Unmarshal(tuple[0], &er.EventID); err != nil {
		return fmt.Errorf("gomatrixserverlib: invalid event reference, first element is invalid: %q %v", string(tuple[0]), err)
	}
	var hashes struct {
		SHA256 Base64String `json:"sha256"`
	}
	if err := json.Unmarshal(tuple[1], &hashes); err != nil {
		return fmt.Errorf("gomatrixserverlib: invalid event reference, second element is invalid: %q %v", string(tuple[1]), err)
	}
	er.EventSHA256 = hashes.SHA256
	return nil
}

// MarshalJSON implements json.Marshaller
func (er EventReference) MarshalJSON() ([]byte, error) {
	hashes := struct {
		SHA256 Base64String `json:"sha256"`
	}{er.EventSHA256}

	tuple := []interface{}{er.EventID, hashes}

	return json.Marshal(&tuple)
}
