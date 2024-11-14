// Copyright 2024 New Vector Ltd.
// Copyright 2019, 2020 The Matrix.org Foundation C.I.C.
// Copyright 2017, 2018 New Vector Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/types"
)

const stateSnapshotSchema = `
-- The state of a room before an event.
-- Stored as a list of state_block entries stored in a separate table.
-- The actual state is constructed by combining all the state_block entries
-- referenced by state_block_nids together. If the same state key tuple appears
-- multiple times then the entry from the later state_block clobbers the earlier
-- entries.
-- This encoding format allows us to implement a delta encoding which is useful
-- because room state tends to accumulate small changes over time. Although if
-- the list of deltas becomes too long it becomes more efficient to encode
-- the full state under single state_block_nid.
CREATE SEQUENCE IF NOT EXISTS roomserver_state_snapshot_nid_seq;
CREATE TABLE IF NOT EXISTS roomserver_state_snapshots (
	-- The state snapshot NID that identifies this snapshot.
	state_snapshot_nid bigint PRIMARY KEY DEFAULT nextval('roomserver_state_snapshot_nid_seq'),
	-- The hash of the state snapshot, which is used to enforce uniqueness. The hash is
	-- generated in Dendrite and passed through to the database, as a btree index over 
	-- this column is cheap and fits within the maximum index size.
	state_snapshot_hash BYTEA UNIQUE,
	-- The room NID that the snapshot belongs to.
	room_nid bigint NOT NULL,
	-- The state blocks contained within this snapshot.
	state_block_nids bigint[] NOT NULL
);
`

// Insert a new state snapshot. If we conflict on the hash column then
// we must perform an update so that the RETURNING statement returns the
// ID of the row that we conflicted with, so that we can then refer to
// the original snapshot.
const insertStateSQL = "" +
	"INSERT INTO roomserver_state_snapshots (state_snapshot_hash, room_nid, state_block_nids)" +
	" VALUES ($1, $2, $3)" +
	" ON CONFLICT (state_snapshot_hash) DO UPDATE SET room_nid=$2" +
	// Performing an update, above, ensures that the RETURNING statement
	// below will always return a valid state snapshot ID
	" RETURNING state_snapshot_nid"

// Bulk state data NID lookup.
// Sorting by state_snapshot_nid means we can use binary search over the result
// to lookup the state data NIDs for a state snapshot NID.
const bulkSelectStateBlockNIDsSQL = "" +
	"SELECT state_snapshot_nid, state_block_nids FROM roomserver_state_snapshots" +
	" WHERE state_snapshot_nid = ANY($1) ORDER BY state_snapshot_nid ASC"

// Looks up both the history visibility event and relevant membership events from
// a given domain name from a given state snapshot. This is used to optimise the
// helpers.CheckServerAllowedToSeeEvent function.
// TODO: There's a sequence scan here because of the hash join strategy, which is
// probably O(n) on state key entries, so there must be a way to avoid that somehow.
// Event type NIDs are:
// - 5: m.room.member as per https://github.com/element-hq/dendrite/blob/c7f7aec4d07d59120d37d5b16a900f6d608a75c4/roomserver/storage/postgres/event_types_table.go#L40
// - 7: m.room.history_visibility as per https://github.com/element-hq/dendrite/blob/c7f7aec4d07d59120d37d5b16a900f6d608a75c4/roomserver/storage/postgres/event_types_table.go#L42
const bulkSelectStateForHistoryVisibilitySQL = `
	SELECT event_nid FROM (
	  SELECT event_nid, event_type_nid, event_state_key_nid FROM roomserver_events
	  WHERE (event_type_nid = 5 OR event_type_nid = 7)
	  AND event_nid = ANY(
	    SELECT UNNEST(event_nids) FROM roomserver_state_block
	    WHERE state_block_nid = ANY(
	      SELECT UNNEST(state_block_nids) FROM roomserver_state_snapshots
	      WHERE state_snapshot_nid = $1
	    )
	  )
	  ORDER BY depth ASC
	) AS roomserver_events
	INNER JOIN roomserver_event_state_keys
	  ON roomserver_events.event_state_key_nid = roomserver_event_state_keys.event_state_key_nid
	  AND (event_type_nid = 7 OR event_state_key LIKE '%:' || $2);
`

// bulkSelectMembershipForHistoryVisibilitySQL is an optimization to get membership events for a specific user for defined set of events.
// Returns the event_id of the event we want the membership event for, the event_id of the membership event and the membership event JSON.
const bulkSelectMembershipForHistoryVisibilitySQL = `
SELECT re.event_id, re2.event_id, rej.event_json
FROM roomserver_events re
LEFT JOIN roomserver_state_snapshots rss on re.state_snapshot_nid = rss.state_snapshot_nid
CROSS JOIN unnest(rss.state_block_nids) AS blocks(block_nid)
LEFT JOIN roomserver_state_block rsb ON rsb.state_block_nid = blocks.block_nid
CROSS JOIN unnest(rsb.event_nids) AS rsb2(event_nid)
JOIN roomserver_events re2 ON re2.room_nid = $3 AND re2.event_type_nid = 5 AND re2.event_nid = rsb2.event_nid AND re2.event_state_key_nid = $1
LEFT JOIN roomserver_event_json rej ON rej.event_nid = re2.event_nid
WHERE re.event_id = ANY($2)

`

type stateSnapshotStatements struct {
	insertStateStmt                               *sql.Stmt
	bulkSelectStateBlockNIDsStmt                  *sql.Stmt
	bulkSelectStateForHistoryVisibilityStmt       *sql.Stmt
	bulktSelectMembershipForHistoryVisibilityStmt *sql.Stmt
}

func CreateStateSnapshotTable(db *sql.DB) error {
	_, err := db.Exec(stateSnapshotSchema)
	return err
}

func PrepareStateSnapshotTable(db *sql.DB) (*stateSnapshotStatements, error) {
	s := &stateSnapshotStatements{}

	return s, sqlutil.StatementList{
		{&s.insertStateStmt, insertStateSQL},
		{&s.bulkSelectStateBlockNIDsStmt, bulkSelectStateBlockNIDsSQL},
		{&s.bulkSelectStateForHistoryVisibilityStmt, bulkSelectStateForHistoryVisibilitySQL},
		{&s.bulktSelectMembershipForHistoryVisibilityStmt, bulkSelectMembershipForHistoryVisibilitySQL},
	}.Prepare(db)
}

func (s *stateSnapshotStatements) InsertState(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, nids types.StateBlockNIDs,
) (stateNID types.StateSnapshotNID, err error) {
	nids = nids[:util.SortAndUnique(nids)]
	err = sqlutil.TxStmt(txn, s.insertStateStmt).QueryRowContext(ctx, nids.Hash(), int64(roomNID), stateBlockNIDsAsArray(nids)).Scan(&stateNID)
	if err != nil {
		return 0, err
	}
	return
}

func (s *stateSnapshotStatements) BulkSelectStateBlockNIDs(
	ctx context.Context, txn *sql.Tx, stateNIDs []types.StateSnapshotNID,
) ([]types.StateBlockNIDList, error) {
	nids := make([]int64, len(stateNIDs))
	for i := range stateNIDs {
		nids[i] = int64(stateNIDs[i])
	}
	stmt := sqlutil.TxStmt(txn, s.bulkSelectStateBlockNIDsStmt)
	rows, err := stmt.QueryContext(ctx, pq.Int64Array(nids))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	results := make([]types.StateBlockNIDList, len(stateNIDs))
	i := 0
	var stateBlockNIDs pq.Int64Array
	for ; rows.Next(); i++ {
		result := &results[i]
		if err = rows.Scan(&result.StateSnapshotNID, &stateBlockNIDs); err != nil {
			return nil, err
		}
		result.StateBlockNIDs = make([]types.StateBlockNID, len(stateBlockNIDs))
		for k := range stateBlockNIDs {
			result.StateBlockNIDs[k] = types.StateBlockNID(stateBlockNIDs[k])
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if i != len(stateNIDs) {
		return nil, types.MissingStateError(fmt.Sprintf("storage: state NIDs missing from the database (%d != %d)", i, len(stateNIDs)))
	}
	return results, nil
}

func (s *stateSnapshotStatements) BulkSelectStateForHistoryVisibility(
	ctx context.Context, txn *sql.Tx, stateSnapshotNID types.StateSnapshotNID, domain string,
) ([]types.EventNID, error) {
	stmt := sqlutil.TxStmt(txn, s.bulkSelectStateForHistoryVisibilityStmt)
	rows, err := stmt.QueryContext(ctx, stateSnapshotNID, domain)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	results := make([]types.EventNID, 0, 16)
	for rows.Next() {
		var eventNID types.EventNID
		if err = rows.Scan(&eventNID); err != nil {
			return nil, err
		}
		results = append(results, eventNID)
	}
	return results, rows.Err()
}

func (s *stateSnapshotStatements) BulkSelectMembershipForHistoryVisibility(
	ctx context.Context, txn *sql.Tx, userNID types.EventStateKeyNID, roomInfo *types.RoomInfo, eventIDs ...string,
) (map[string]*types.HeaderedEvent, error) {
	stmt := sqlutil.TxStmt(txn, s.bulktSelectMembershipForHistoryVisibilityStmt)
	rows, err := stmt.QueryContext(ctx, userNID, pq.Array(eventIDs), roomInfo.RoomNID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	result := make(map[string]*types.HeaderedEvent, len(eventIDs))
	var evJson []byte
	var eventID string
	var membershipEventID string

	knownEvents := make(map[string]*types.HeaderedEvent, len(eventIDs))
	verImpl, err := gomatrixserverlib.GetRoomVersion(roomInfo.RoomVersion)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		if err = rows.Scan(&eventID, &membershipEventID, &evJson); err != nil {
			return nil, err
		}
		if len(evJson) == 0 {
			result[eventID] = &types.HeaderedEvent{}
			continue
		}
		// If we already know this event, don't try to marshal the json again
		if ev, ok := knownEvents[membershipEventID]; ok {
			result[eventID] = ev
			continue
		}
		event, err := verImpl.NewEventFromTrustedJSON(evJson, false)
		if err != nil {
			result[eventID] = &types.HeaderedEvent{}
			// not fatal
			continue
		}
		he := &types.HeaderedEvent{PDU: event}
		result[eventID] = he
		knownEvents[membershipEventID] = he
	}
	return result, rows.Err()
}
