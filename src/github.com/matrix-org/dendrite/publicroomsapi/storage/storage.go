// Copyright 2017 Vector Creations Ltd
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

package storage

import (
	"database/sql"
	"encoding/json"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/publicroomsapi/types"

	"github.com/matrix-org/gomatrixserverlib"
)

// PublicRoomsServerDatabase represents a public rooms server database.
type PublicRoomsServerDatabase struct {
	db         *sql.DB
	partitions common.PartitionOffsetStatements
	statements publicRoomsStatements
}

type attributeValue interface{}

// NewPublicRoomsServerDatabase creates a new public rooms server database.
func NewPublicRoomsServerDatabase(dataSourceName string) (*PublicRoomsServerDatabase, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	partitions := common.PartitionOffsetStatements{}
	if err = partitions.Prepare(db, "publicroomsapi"); err != nil {
		return nil, err
	}
	statements := publicRoomsStatements{}
	if err = statements.prepare(db); err != nil {
		return nil, err
	}
	return &PublicRoomsServerDatabase{db, partitions, statements}, nil
}

// PartitionOffsets implements common.PartitionStorer
func (d *PublicRoomsServerDatabase) PartitionOffsets(topic string) ([]common.PartitionOffset, error) {
	return d.partitions.SelectPartitionOffsets(topic)
}

// SetPartitionOffset implements common.PartitionStorer
func (d *PublicRoomsServerDatabase) SetPartitionOffset(topic string, partition int32, offset int64) error {
	return d.partitions.UpsertPartitionOffset(topic, partition, offset)
}

// GetRoomVisibility returns the room visibility as a boolean: true if the room
// is publicly visible, false if not.
// Returns an error if the retrieval failed.
func (d *PublicRoomsServerDatabase) GetRoomVisibility(roomID string) (bool, error) {
	return d.statements.selectRoomVisibility(roomID)
}

// SetRoomVisibility updates the visibility attribute of a room. This attribute
// must be set to true if the room is publicly visible, false if not.
// Returns an error if the update failed.
func (d *PublicRoomsServerDatabase) SetRoomVisibility(visible bool, roomID string) error {
	return d.statements.updateRoomAttribute("visibility", visible, roomID)
}

// CountPublicRooms returns the number of room set as publicly visible on the server.
// Returns an error if the retrieval failed.
func (d *PublicRoomsServerDatabase) CountPublicRooms() (int64, error) {
	return d.statements.countPublicRooms()
}

// GetPublicRooms returns an array containing the local rooms set as publicly visible, ordered by their number
// of joined members. This array can be limited by a given number of elements, and offset by a given value.
// If the limit is 0, doesn't limit the number of results. If the offset is 0 too, the array contains all
// the rooms set as publicly visible on the server.
// Returns an error if the retrieval failed.
func (d *PublicRoomsServerDatabase) GetPublicRooms(offset int64, limit int16, filter string) ([]types.PublicRoom, error) {
	return d.statements.selectPublicRooms(offset, limit, filter)
}

// UpdateRoomFromEvents iterate over a slice of state events and call
// UpdateRoomFromEvent on each of them to update the database representation of
// the rooms updated by each event.
// The slice of events to remove is used to update the number of joined members
// for the room in the database.
// If the update triggered by one of the events failed, aborts the process and
// returns an error.
func (d *PublicRoomsServerDatabase) UpdateRoomFromEvents(
	eventsToAdd []gomatrixserverlib.Event, eventsToRemove []gomatrixserverlib.Event,
) error {
	for _, event := range eventsToAdd {
		if err := d.UpdateRoomFromEvent(event); err != nil {
			return err
		}
	}

	for _, event := range eventsToRemove {
		if event.Type() == "m.room.member" {
			if err := d.updateNumJoinedUsers(event, true); err != nil {
				return err
			}
		}
	}

	return nil
}

// UpdateRoomFromEvent updates the database representation of a room from a Matrix event, by
// checking the event's type to know which attribute to change and using the event's content
// to define the new value of the attribute.
// If the event doesn't match with any property used to compute the public room directory,
// does nothing.
// If something went wrong during the process, returns an error.
func (d *PublicRoomsServerDatabase) UpdateRoomFromEvent(event gomatrixserverlib.Event) error {
	roomID := event.RoomID()

	// Process the event according to its type
	switch event.Type() {
	case "m.room.create":
		return d.statements.insertNewRoom(roomID)
	case "m.room.member":
		return d.updateNumJoinedUsers(event, false)
	case "m.room.aliases":
		return d.updateRoomAliases(event, roomID)
	case "m.room.canonical_alias":
		var content common.CanonicalAliasContent
		field := &(content.Alias)
		attrName := "canonical_alias"
		return d.updateStringAttribute(attrName, event, &content, field, roomID)
	case "m.room.name":
		var content common.NameContent
		field := &(content.Name)
		attrName := "name"
		return d.updateStringAttribute(attrName, event, &content, field, roomID)
	case "m.room.topic":
		var content common.TopicContent
		field := &(content.Topic)
		attrName := "topic"
		return d.updateStringAttribute(attrName, event, &content, field, roomID)
	case "m.room.avatar":
		var content common.AvatarContent
		field := &(content.URL)
		attrName := "avatar_url"
		return d.updateStringAttribute(attrName, event, &content, field, roomID)
	case "m.room.history_visibility":
		var content common.HistoryVisibilityContent
		field := &(content.HistoryVisibility)
		attrName := "world_readable"
		strForTrue := "world_readable"
		return d.updateBooleanAttribute(attrName, event, &content, field, strForTrue, roomID)
	case "m.room.guest_access":
		var content common.GuestAccessContent
		field := &(content.GuestAccess)
		attrName := "guest_can_join"
		strForTrue := "can_join"
		return d.updateBooleanAttribute(attrName, event, &content, field, strForTrue, roomID)
	}

	// If the event type didn't match, return with no error
	return nil
}

// updateNumJoinedUsers updates the number of joined user in the database representation
// of the room identified by a given room ID, using a given "m.room.member" Matrix event.
// If the membership property of the event isn't "join", ignores it and returs nil.
// If the remove parameter is set to false, increments the joined members counter in the
// database, if set to truem decrements it.
// Returns an error if the update failed.
func (d *PublicRoomsServerDatabase) updateNumJoinedUsers(
	membershipEvent gomatrixserverlib.Event, remove bool,
) error {
	membership, err := membershipEvent.Membership()
	if err != nil {
		return err
	}

	if membership != "join" {
		return nil
	}

	if remove {
		return d.statements.decrementJoinedMembersInRoom(membershipEvent.RoomID())
	}
	return d.statements.incrementJoinedMembersInRoom(membershipEvent.RoomID())
}

// updateStringAttribute updates a given string attribute in the database
// representation of the room identified by a given ID, using a given string data
// field from content of the Matrix event triggering the update.
// Returns an error if decoding the Matrix event's content or updating the attribute
// failed.
func (d *PublicRoomsServerDatabase) updateStringAttribute(
	attrName string, event gomatrixserverlib.Event, content interface{},
	field *string, roomID string,
) error {
	if err := json.Unmarshal(event.Content(), content); err != nil {
		return err
	}

	return d.statements.updateRoomAttribute(attrName, *field, roomID)
}

// updateBooleanAttribute updates a given boolean attribute in the database
// representation of the room identified by a given ID, using a given string data
// field from content of the Matrix event triggering the update.
// The attribute is set to true if the field matches a given string, false if not.
// Returns an error if decoding the Matrix event's content or updating the attribute
// failed.
func (d *PublicRoomsServerDatabase) updateBooleanAttribute(
	attrName string, event gomatrixserverlib.Event, content interface{},
	field *string, strForTrue string, roomID string,
) error {
	if err := json.Unmarshal(event.Content(), content); err != nil {
		return err
	}

	var attrValue bool
	if *field == strForTrue {
		attrValue = true
	} else {
		attrValue = false
	}

	return d.statements.updateRoomAttribute(attrName, attrValue, roomID)
}

// updateRoomAliases decodes the content of a "m.room.aliases" Matrix event and update the list of aliases of
// a given room with it.
// Returns an error if decoding the Matrix event or updating the list failed.
func (d *PublicRoomsServerDatabase) updateRoomAliases(aliasesEvent gomatrixserverlib.Event, roomID string) error {
	var content common.AliasesContent
	if err := json.Unmarshal(aliasesEvent.Content(), &content); err != nil {
		return err
	}

	return d.statements.updateRoomAttribute("aliases", content.Aliases, roomID)
}
