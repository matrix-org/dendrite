// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/matrix-org/dendrite/common"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/publicroomsapi/types"
)

var editableAttributes = []string{
	"aliases",
	"canonical_alias",
	"name",
	"topic",
	"world_readable",
	"guest_can_join",
	"avatar_url",
	"visibility",
}

const publicRoomsSchema = `
-- Stores all of the rooms with data needed to create the server's room directory
CREATE TABLE IF NOT EXISTS publicroomsapi_public_rooms(
	-- The room's ID
	room_id TEXT NOT NULL PRIMARY KEY,
	-- Number of joined members in the room
	joined_members INTEGER NOT NULL DEFAULT 0,
	-- Aliases of the room (empty array if none)
	aliases TEXT[] NOT NULL DEFAULT '{}'::TEXT[],
	-- Canonical alias of the room (empty string if none)
	canonical_alias TEXT NOT NULL DEFAULT '',
	-- Name of the room (empty string if none)
	name TEXT NOT NULL DEFAULT '',
	-- Topic of the room (empty string if none)
	topic TEXT NOT NULL DEFAULT '',
	-- Is the room world readable?
	world_readable BOOLEAN NOT NULL DEFAULT false,
	-- Can guest join the room?
	guest_can_join BOOLEAN NOT NULL DEFAULT false,
	-- URL of the room avatar (empty string if none)
	avatar_url TEXT NOT NULL DEFAULT '',
	-- Visibility of the room: true means the room is publicly visible, false
	-- means the room is private
	visibility BOOLEAN NOT NULL DEFAULT false
);
`

const countPublicRoomsSQL = "" +
	"SELECT COUNT(*) FROM publicroomsapi_public_rooms" +
	" WHERE visibility = true"

const selectPublicRoomsSQL = "" +
	"SELECT room_id, joined_members, aliases, canonical_alias, name, topic, world_readable, guest_can_join, avatar_url" +
	" FROM publicroomsapi_public_rooms WHERE visibility = true" +
	" ORDER BY joined_members DESC" +
	" OFFSET $1"

const selectPublicRoomsWithLimitSQL = "" +
	"SELECT room_id, joined_members, aliases, canonical_alias, name, topic, world_readable, guest_can_join, avatar_url" +
	" FROM publicroomsapi_public_rooms WHERE visibility = true" +
	" ORDER BY joined_members DESC" +
	" OFFSET $1 LIMIT $2"

const selectPublicRoomsWithFilterSQL = "" +
	"SELECT room_id, joined_members, aliases, canonical_alias, name, topic, world_readable, guest_can_join, avatar_url" +
	" FROM publicroomsapi_public_rooms" +
	" WHERE visibility = true" +
	" AND (LOWER(name) LIKE LOWER($1)" +
	" OR LOWER(topic) LIKE LOWER($1)" +
	" OR LOWER(ARRAY_TO_STRING(aliases, ',')) LIKE LOWER($1))" +
	" ORDER BY joined_members DESC" +
	" OFFSET $2"

const selectPublicRoomsWithLimitAndFilterSQL = "" +
	"SELECT room_id, joined_members, aliases, canonical_alias, name, topic, world_readable, guest_can_join, avatar_url" +
	" FROM publicroomsapi_public_rooms" +
	" WHERE visibility = true" +
	" AND (LOWER(name) LIKE LOWER($1)" +
	" OR LOWER(topic) LIKE LOWER($1)" +
	" OR LOWER(ARRAY_TO_STRING(aliases, ',')) LIKE LOWER($1))" +
	" ORDER BY joined_members DESC" +
	" OFFSET $2 LIMIT $3"

const selectRoomVisibilitySQL = "" +
	"SELECT visibility FROM publicroomsapi_public_rooms" +
	" WHERE room_id = $1"

const insertNewRoomSQL = "" +
	"INSERT INTO publicroomsapi_public_rooms(room_id)" +
	" VALUES ($1)"

const incrementJoinedMembersInRoomSQL = "" +
	"UPDATE publicroomsapi_public_rooms" +
	" SET joined_members = joined_members + 1" +
	" WHERE room_id = $1"

const decrementJoinedMembersInRoomSQL = "" +
	"UPDATE publicroomsapi_public_rooms" +
	" SET joined_members = joined_members - 1" +
	" WHERE room_id = $1"

const updateRoomAttributeSQL = "" +
	"UPDATE publicroomsapi_public_rooms" +
	" SET %s = $1" +
	" WHERE room_id = $2"

type publicRoomsStatements struct {
	countPublicRoomsStmt                    *sql.Stmt
	selectPublicRoomsStmt                   *sql.Stmt
	selectPublicRoomsWithLimitStmt          *sql.Stmt
	selectPublicRoomsWithFilterStmt         *sql.Stmt
	selectPublicRoomsWithLimitAndFilterStmt *sql.Stmt
	selectRoomVisibilityStmt                *sql.Stmt
	insertNewRoomStmt                       *sql.Stmt
	incrementJoinedMembersInRoomStmt        *sql.Stmt
	decrementJoinedMembersInRoomStmt        *sql.Stmt
	updateRoomAttributeStmts                map[string]*sql.Stmt
}

func (s *publicRoomsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(publicRoomsSchema)
	if err != nil {
		return
	}

	stmts := statementList{
		{&s.countPublicRoomsStmt, countPublicRoomsSQL},
		{&s.selectPublicRoomsStmt, selectPublicRoomsSQL},
		{&s.selectPublicRoomsWithLimitStmt, selectPublicRoomsWithLimitSQL},
		{&s.selectPublicRoomsWithFilterStmt, selectPublicRoomsWithFilterSQL},
		{&s.selectPublicRoomsWithLimitAndFilterStmt, selectPublicRoomsWithLimitAndFilterSQL},
		{&s.selectRoomVisibilityStmt, selectRoomVisibilitySQL},
		{&s.insertNewRoomStmt, insertNewRoomSQL},
		{&s.incrementJoinedMembersInRoomStmt, incrementJoinedMembersInRoomSQL},
		{&s.decrementJoinedMembersInRoomStmt, decrementJoinedMembersInRoomSQL},
	}

	if err = stmts.prepare(db); err != nil {
		return
	}

	s.updateRoomAttributeStmts = make(map[string]*sql.Stmt)
	for _, editable := range editableAttributes {
		stmt := fmt.Sprintf(updateRoomAttributeSQL, editable)
		if s.updateRoomAttributeStmts[editable], err = db.Prepare(stmt); err != nil {
			return
		}
	}

	return
}

func (s *publicRoomsStatements) countPublicRooms(ctx context.Context) (nb int64, err error) {
	err = s.countPublicRoomsStmt.QueryRowContext(ctx).Scan(&nb)
	return
}

func (s *publicRoomsStatements) selectPublicRooms(
	ctx context.Context, offset int64, limit int16, filter string,
) ([]types.PublicRoom, error) {
	var rows *sql.Rows
	var err error

	if len(filter) > 0 {
		pattern := "%" + filter + "%"
		if limit == 0 {
			rows, err = s.selectPublicRoomsWithFilterStmt.QueryContext(
				ctx, pattern, offset,
			)
		} else {
			rows, err = s.selectPublicRoomsWithLimitAndFilterStmt.QueryContext(
				ctx, pattern, offset, limit,
			)
		}
	} else {
		if limit == 0 {
			rows, err = s.selectPublicRoomsStmt.QueryContext(ctx, offset)
		} else {
			rows, err = s.selectPublicRoomsWithLimitStmt.QueryContext(
				ctx, offset, limit,
			)
		}
	}

	if err != nil {
		return []types.PublicRoom{}, nil
	}
	defer common.LogIfError(ctx, rows.Close(), "selectPublicRooms: rows.close() failed")

	rooms := []types.PublicRoom{}
	for rows.Next() {
		var r types.PublicRoom
		var aliases pq.StringArray

		err = rows.Scan(
			&r.RoomID, &r.NumJoinedMembers, &aliases, &r.CanonicalAlias,
			&r.Name, &r.Topic, &r.WorldReadable, &r.GuestCanJoin, &r.AvatarURL,
		)
		if err != nil {
			return rooms, err
		}

		r.Aliases = aliases

		rooms = append(rooms, r)
	}

	return rooms, rows.Err()
}

func (s *publicRoomsStatements) selectRoomVisibility(
	ctx context.Context, roomID string,
) (v bool, err error) {
	err = s.selectRoomVisibilityStmt.QueryRowContext(ctx, roomID).Scan(&v)
	return
}

func (s *publicRoomsStatements) insertNewRoom(
	ctx context.Context, roomID string,
) error {
	_, err := s.insertNewRoomStmt.ExecContext(ctx, roomID)
	return err
}

func (s *publicRoomsStatements) incrementJoinedMembersInRoom(
	ctx context.Context, roomID string,
) error {
	_, err := s.incrementJoinedMembersInRoomStmt.ExecContext(ctx, roomID)
	return err
}

func (s *publicRoomsStatements) decrementJoinedMembersInRoom(
	ctx context.Context, roomID string,
) error {
	_, err := s.decrementJoinedMembersInRoomStmt.ExecContext(ctx, roomID)
	return err
}

func (s *publicRoomsStatements) updateRoomAttribute(
	ctx context.Context, attrName string, attrValue attributeValue, roomID string,
) error {
	stmt, isEditable := s.updateRoomAttributeStmts[attrName]

	if !isEditable {
		return errors.New("Cannot edit " + attrName)
	}

	var value interface{}
	switch v := attrValue.(type) {
	case []string:
		value = pq.StringArray(v)
	case bool, string:
		value = attrValue
	default:
		return errors.New("Unsupported attribute type, must be bool, string or []string")
	}

	_, err := stmt.ExecContext(ctx, value, roomID)
	return err
}
