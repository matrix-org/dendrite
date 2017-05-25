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
	"strconv"
	"strings"
)

// createContent is the JSON content of a m.room.create event along with
// the top level keys needed for auth.
// See https://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-create for descriptions of the fields.
type createContent struct {
	// We need the domain of the create event when checking federatability.
	senderDomain string
	// We need the roomID to check that events are in the same room as the create event.
	roomID string
	// We need the eventID to check the first join event in the room.
	eventID string
	// The "m.federate" flag tells us whether the room can be federated to other servers.
	Federate *bool `json:"m.federate"`
	// The creator of the room tells us what the default power levels are.
	Creator string `json:"creator"`
}

// newCreateContentFromAuthEvents loads the create event content from the create event in the
// auth events.
func newCreateContentFromAuthEvents(authEvents AuthEventProvider) (c createContent, err error) {
	var createEvent *Event
	if createEvent, err = authEvents.Create(); err != nil {
		return
	}
	if createEvent == nil {
		err = errorf("missing create event")
		return
	}
	if err = json.Unmarshal(createEvent.Content(), &c); err != nil {
		err = errorf("unparsable create event content: %s", err.Error())
		return
	}
	c.roomID = createEvent.RoomID()
	c.eventID = createEvent.EventID()
	if c.senderDomain, err = domainFromID(createEvent.Sender()); err != nil {
		return
	}
	return
}

// domainAllowed checks whether the domain is allowed in the room by the
// "m.federate" flag.
func (c *createContent) domainAllowed(domain string) error {
	if domain == c.senderDomain {
		// If the domain matches the domain of the create event then the event
		// is always allowed regardless of the value of the "m.federate" flag.
		return nil
	}
	if c.Federate == nil || *c.Federate {
		// The m.federate field defaults to true.
		// If the domains are different then event is only allowed if the
		// "m.federate" flag is absent or true.
		return nil
	}
	return errorf("room is unfederatable")
}

// userIDAllowed checks whether the domain part of the user ID is allowed in
// the room by the "m.federate" flag.
func (c *createContent) userIDAllowed(id string) error {
	domain, err := domainFromID(id)
	if err != nil {
		return err
	}
	return c.domainAllowed(domain)
}

// domainFromID returns everything after the first ":" character to extract
// the domain part of a matrix ID.
func domainFromID(id string) (string, error) {
	// IDs have the format: SIGIL LOCALPART ":" DOMAIN
	// Split on the first ":" character since the domain can contain ":"
	// characters.
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		// The ID must have a ":" character.
		return "", errorf("invalid ID: %q", id)
	}
	// Return everything after the first ":" character.
	return parts[1], nil
}

// memberContent is the JSON content of a m.room.member event needed for auth checks.
// See https://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-member for descriptions of the fields.
type memberContent struct {
	// We use the membership key in order to check if the user is in the room.
	Membership string `json:"membership"`
	// We use the third_party_invite key to special case thirdparty invites.
	ThirdPartyInvite rawJSON `json:"third_party_invite,omitempty"`
}

// newMemberContentFromAuthEvents loads the member content from the member event for the user ID in the auth events.
// Returns an error if there was an error loading the member event or parsing the event content.
func newMemberContentFromAuthEvents(authEvents AuthEventProvider, userID string) (c memberContent, err error) {
	var memberEvent *Event
	if memberEvent, err = authEvents.Member(userID); err != nil {
		return
	}
	if memberEvent == nil {
		// If there isn't a member event then the membership for the user
		// defaults to leave.
		c.Membership = leave
		return
	}
	return newMemberContentFromEvent(*memberEvent)
}

// newMemberContentFromEvent parse the member content from an event.
// Returns an error if the content couldn't be parsed.
func newMemberContentFromEvent(event Event) (c memberContent, err error) {
	if err = json.Unmarshal(event.Content(), &c); err != nil {
		err = errorf("unparsable member event content: %s", err.Error())
		return
	}
	return
}

// joinRuleContent is the JSON content of a m.room.join_rules event needed for auth checks.
// See  https://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-join-rules for descriptions of the fields.
type joinRuleContent struct {
	// We use the join_rule key to check whether join m.room.member events are allowed.
	JoinRule string `json:"join_rule"`
}

// newJoinRuleContentFromAuthEvents loads the join rule content from the join rules event in the auth event.
// Returns an error if there was an error loading the join rule event or parsing the content.
func newJoinRuleContentFromAuthEvents(authEvents AuthEventProvider) (c joinRuleContent, err error) {
	var joinRulesEvent *Event
	if joinRulesEvent, err = authEvents.JoinRules(); err != nil {
		return
	}
	if joinRulesEvent == nil {
		// Default to "invite"
		// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L368
		c.JoinRule = invite
		return
	}
	if err = json.Unmarshal(joinRulesEvent.Content(), &c); err != nil {
		err = errorf("unparsable join_rules event content: %s", err.Error())
		return
	}
	return
}

// powerLevelContent is the JSON content of a m.room.power_levels event needed for auth checks.
// We can't unmarshal the content directly from JSON because we need to set
// defaults and convert string values to int values.
// See https://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-power-levels for descriptions of the fields.
type powerLevelContent struct {
	banLevel          int64
	inviteLevel       int64
	kickLevel         int64
	redactLevel       int64
	userLevels        map[string]int64
	userDefaultLevel  int64
	eventLevels       map[string]int64
	eventDefaultLevel int64
	stateDefaultLevel int64
}

// userLevel returns the power level a user has in the room.
func (c *powerLevelContent) userLevel(userID string) int64 {
	level, ok := c.userLevels[userID]
	if ok {
		return level
	}
	return c.userDefaultLevel
}

// eventLevel returns the power level needed to send an event in the room.
func (c *powerLevelContent) eventLevel(eventType string, isState bool) int64 {
	if eventType == MRoomThirdPartyInvite {
		// Special case third_party_invite events to have the same level as
		// m.room.member invite events.
		// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L182
		return c.inviteLevel
	}
	level, ok := c.eventLevels[eventType]
	if ok {
		return level
	}
	if isState {
		return c.stateDefaultLevel
	}
	return c.eventDefaultLevel
}

// newPowerLevelContentFromAuthEvents loads the power level content from the
// power level event in the auth events or returns the default values if there
// is no power level event.
func newPowerLevelContentFromAuthEvents(authEvents AuthEventProvider, creatorUserID string) (c powerLevelContent, err error) {
	powerLevelsEvent, err := authEvents.PowerLevels()
	if err != nil {
		return
	}
	if powerLevelsEvent != nil {
		return newPowerLevelContentFromEvent(*powerLevelsEvent)
	}

	// If there are no power levels then fall back to defaults.
	c.defaults()
	// If there is no power level event then the creator gets level 100
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L569
	c.userLevels = map[string]int64{creatorUserID: 100}
	return
}

// defaults sets the power levels to their default values.
func (c *powerLevelContent) defaults() {
	// Default invite level is 0.
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L426
	c.inviteLevel = 0
	// Default ban, kick and redacts levels are 50
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L376
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L456
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L1041
	c.banLevel = 50
	c.kickLevel = 50
	c.redactLevel = 50
	// Default user level is 0
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L558
	c.userDefaultLevel = 0
	// Default event level is 0, Default state level is 50
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L987
	// https://github.com/matrix-org/synapse/blob/v0.18.5/synapse/api/auth.py#L991
	c.eventDefaultLevel = 0
	c.stateDefaultLevel = 50

}

// newPowerLevelContentFromEvent loads the power level content from an event.
func newPowerLevelContentFromEvent(event Event) (c powerLevelContent, err error) {
	// Set the levels to their default values.
	c.defaults()

	// We can't extract the JSON directly to the powerLevelContent because we
	// need to convert string values to int values.
	var content struct {
		InviteLevel       levelJSONValue            `json:"invite"`
		BanLevel          levelJSONValue            `json:"ban"`
		KickLevel         levelJSONValue            `json:"kick"`
		RedactLevel       levelJSONValue            `json:"redact"`
		UserLevels        map[string]levelJSONValue `json:"users"`
		UsersDefaultLevel levelJSONValue            `json:"users_default"`
		EventLevels       map[string]levelJSONValue `json:"events"`
		StateDefaultLevel levelJSONValue            `json:"state_default"`
		EventDefaultLevel levelJSONValue            `json:"event_default"`
	}
	if err = json.Unmarshal(event.Content(), &content); err != nil {
		err = errorf("unparsable power_levels event content: %s", err.Error())
		return
	}

	// Update the levels with the values that are present in the event content.
	content.InviteLevel.assignIfExists(&c.inviteLevel)
	content.BanLevel.assignIfExists(&c.banLevel)
	content.KickLevel.assignIfExists(&c.kickLevel)
	content.RedactLevel.assignIfExists(&c.redactLevel)
	content.UsersDefaultLevel.assignIfExists(&c.userDefaultLevel)
	content.StateDefaultLevel.assignIfExists(&c.stateDefaultLevel)
	content.EventDefaultLevel.assignIfExists(&c.eventDefaultLevel)

	for k, v := range content.UserLevels {
		if c.userLevels == nil {
			c.userLevels = make(map[string]int64)
		}
		c.userLevels[k] = v.value
	}

	for k, v := range content.EventLevels {
		if c.eventLevels == nil {
			c.eventLevels = make(map[string]int64)
		}
		c.eventLevels[k] = v.value
	}

	return
}

// A levelJSONValue is used for unmarshalling power levels from JSON.
// It is intended to replicate the effects of x = int(content["key"]) in python.
type levelJSONValue struct {
	// Was a value loaded from the JSON?
	exists bool
	// The integer value of the power level.
	value int64
}

func (v *levelJSONValue) UnmarshalJSON(data []byte) error {
	var stringValue string
	var int64Value int64
	var floatValue float64
	var err error

	// First try to unmarshal as an int64.
	if err = json.Unmarshal(data, &int64Value); err != nil {
		// If unmarshalling as an int64 fails try as a string.
		if err = json.Unmarshal(data, &stringValue); err != nil {
			// If unmarshalling as a string fails try as a float.
			if err = json.Unmarshal(data, &floatValue); err != nil {
				return err
			}
			int64Value = int64(floatValue)
		} else {
			// If we managed to get a string, try parsing the string as an int.
			int64Value, err = strconv.ParseInt(stringValue, 10, 64)
			if err != nil {
				return err
			}
		}
	}
	v.exists = true
	v.value = int64Value
	return nil
}

// assign the power level if a value was present in the JSON.
func (v *levelJSONValue) assignIfExists(to *int64) {
	if v.exists {
		*to = v.value
	}
}

// Check if the user ID is a valid user ID.
func isValidUserID(userID string) bool {
	// TODO: Do we want to add anymore checks beyond checking the sigil and that it has a domain part?
	return userID[0] == '@' && strings.IndexByte(userID, ':') != -1
}
