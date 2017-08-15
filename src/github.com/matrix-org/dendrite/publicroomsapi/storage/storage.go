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

// PublicRoomsServerDatabase represents a public rooms server database
type PublicRoomsServerDatabase struct {
	db         *sql.DB
	partitions common.PartitionOffsetStatements
	statements publicRoomsStatements
}

type attributeValue interface{}

// NewPublicRoomsServerDatabase creates a new public rooms server database
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
func (d *PublicRoomsServerDatabase) GetPublicRooms(offset int64, limit int16) ([]types.PublicRoom, error) {
	return d.statements.selectPublicRooms(offset, limit)
}

// UpdateRoomFromEvent updates the database representation of a room from a Matrix event, by
// checking the event's type to know which attribute to change and using the event's content
// to define the new value of the attribute.
// If the event doesn't match with any property used to compute the public room directory,
// does nothing.
// If something went wrong during the process, returns an error.
func (d *PublicRoomsServerDatabase) UpdateRoomFromEvent(event gomatrixserverlib.Event) error {
	var attrName string
	var attrValue attributeValue

	roomID := event.RoomID()
	switch event.Type() {
	case "m.room.create":
		return d.statements.insertNewRoom(roomID)
	case "m.room.aliases":
		var content common.AliasesContent
		if err := json.Unmarshal(event.Content(), &content); err != nil {
			return err
		}

		attrName = "aliases"
		attrValue = content.Aliases
		break
	case "m.room.canonical_alias":
		var content common.CanonicalAliasContent
		if err := json.Unmarshal(event.Content(), &content); err != nil {
			return err
		}

		attrName = "canonical_alias"
		attrValue = content.Alias
		break
	}

	return d.statements.updateRoomAttribute(attrName, attrValue, roomID)
}
