// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package deltas

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type stateSnapshotData struct {
	StateSnapshotNID types.StateSnapshotNID
	RoomNID          types.RoomNID
}

type stateBlockData struct {
	stateSnapshotData
	StateBlockNID types.StateBlockNID
	EventNIDs     types.EventNIDs
}

func LoadStateBlocksRefactor(m *sqlutil.Migrations) {
	m.AddMigration(UpStateBlocksRefactor, DownStateBlocksRefactor)
}

// nolint:gocyclo
func UpStateBlocksRefactor(tx *sql.Tx) error {
	logrus.Warn("Performing state storage upgrade. Please wait, this may take some time!")
	defer logrus.Warn("State storage upgrade complete")

	var snapshotcount int
	var maxsnapshotid int
	var maxblockid int
	if err := tx.QueryRow(`SELECT COUNT(DISTINCT state_snapshot_nid) FROM roomserver_state_snapshots;`).Scan(&snapshotcount); err != nil {
		return fmt.Errorf("tx.QueryRow.Scan (count snapshots): %w", err)
	}
	if err := tx.QueryRow(`SELECT COALESCE(MAX(state_snapshot_nid),0) FROM roomserver_state_snapshots;`).Scan(&maxsnapshotid); err != nil {
		return fmt.Errorf("tx.QueryRow.Scan (count snapshots): %w", err)
	}
	if err := tx.QueryRow(`SELECT COALESCE(MAX(state_block_nid),0) FROM roomserver_state_block;`).Scan(&maxblockid); err != nil {
		return fmt.Errorf("tx.QueryRow.Scan (count snapshots): %w", err)
	}
	maxsnapshotid++
	maxblockid++

	if _, err := tx.Exec(`ALTER TABLE roomserver_state_block RENAME TO _roomserver_state_block;`); err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE roomserver_state_snapshots RENAME TO _roomserver_state_snapshots;`); err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	// We create new sequences starting with the maximum state snapshot and block NIDs.
	// This means that all newly created snapshots and blocks by the migration will have
	// NIDs higher than these values, so that when we come to update the references to
	// these NIDs using UPDATE statements, we can guarantee we are only ever updating old
	// values and not accidentally overwriting new ones.
	if _, err := tx.Exec(fmt.Sprintf(`CREATE SEQUENCE roomserver_state_block_nid_sequence START WITH %d;`, maxblockid)); err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	if _, err := tx.Exec(fmt.Sprintf(`CREATE SEQUENCE roomserver_state_snapshot_nid_sequence START WITH %d;`, maxsnapshotid)); err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS roomserver_state_block (
			state_block_nid bigint PRIMARY KEY DEFAULT nextval('roomserver_state_block_nid_sequence'),
			state_block_hash BYTEA UNIQUE,
			event_nids bigint[] NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("tx.Exec (create blocks table): %w", err)
	}
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS roomserver_state_snapshots (
			state_snapshot_nid bigint PRIMARY KEY DEFAULT nextval('roomserver_state_snapshot_nid_sequence'),
			state_snapshot_hash BYTEA UNIQUE,
			room_nid bigint NOT NULL,
			state_block_nids bigint[] NOT NULL
		);
	`)
	if err != nil {
		return fmt.Errorf("tx.Exec (create snapshots table): %w", err)
	}
	logrus.Warn("New tables created...")
	// some m.room.create events have a state snapshot but no state blocks at all which makes
	// sense as there is no state before creation. The correct form should be to give the event
	// in question a state snapshot NID of 0 to indicate 'no snapshot'.
	// If we don't do this, we'll fail the assertions later on which try to ensure we didn't forget
	// any snapshots.
	_, err = tx.Exec(
		`UPDATE roomserver_events SET state_snapshot_nid = 0 WHERE event_type_nid = $1 AND event_state_key_nid = $2`,
		types.MRoomCreateNID, types.EmptyStateKeyNID,
	)
	if err != nil {
		return fmt.Errorf("resetting create events snapshots to 0 errored: %s", err)
	}

	batchsize := 100
	for batchoffset := 0; batchoffset < snapshotcount; batchoffset += batchsize {
		var snapshotrows *sql.Rows
		snapshotrows, err = tx.Query(`
			SELECT
				state_snapshot_nid,
				room_nid,
				state_block_nid,
				ARRAY_AGG(event_nid) AS event_nids
			FROM (
				SELECT
					_roomserver_state_snapshots.state_snapshot_nid,
					_roomserver_state_snapshots.room_nid,
					_roomserver_state_block.state_block_nid,
					_roomserver_state_block.event_nid
				FROM
					_roomserver_state_snapshots
					LEFT JOIN _roomserver_state_block ON _roomserver_state_block.state_block_nid = ANY (_roomserver_state_snapshots.state_block_nids)
				WHERE
					_roomserver_state_snapshots.state_snapshot_nid = ANY (
						SELECT
							_roomserver_state_snapshots.state_snapshot_nid
						FROM
							_roomserver_state_snapshots
						ORDER BY _roomserver_state_snapshots.state_snapshot_nid ASC
						LIMIT $1 OFFSET $2
					)
			) AS _roomserver_state_block
			GROUP BY
				state_snapshot_nid,
				room_nid,
				state_block_nid;
		`, batchsize, batchoffset)
		if err != nil {
			return fmt.Errorf("tx.Query: %w", err)
		}

		logrus.Warnf("Rewriting snapshots %d-%d of %d...", batchoffset, batchoffset+batchsize, snapshotcount)
		var snapshots []stateBlockData

		var badCreateSnapshots []stateBlockData
		for snapshotrows.Next() {
			var snapshot stateBlockData
			var eventsarray []sql.NullInt64
			var nulStateBlockNID sql.NullInt64
			if err = snapshotrows.Scan(&snapshot.StateSnapshotNID, &snapshot.RoomNID, &nulStateBlockNID, pq.Array(&eventsarray)); err != nil {
				return fmt.Errorf("rows.Scan: %w", err)
			}
			if nulStateBlockNID.Valid {
				snapshot.StateBlockNID = types.StateBlockNID(nulStateBlockNID.Int64)
			}
			// Dendrite v0.1.0 would not make a state block for the create event, resulting in [NULL] from the query above.
			// Remember the snapshot and we'll fill it in after we close this cursor as we can't have 2 queries running at the same time
			if len(eventsarray) == 1 && !eventsarray[0].Valid {
				badCreateSnapshots = append(badCreateSnapshots, snapshot)
				continue
			}
			for _, e := range eventsarray {
				if e.Valid {
					snapshot.EventNIDs = append(snapshot.EventNIDs, types.EventNID(e.Int64))
				}
			}
			snapshot.EventNIDs = snapshot.EventNIDs[:util.SortAndUnique(snapshot.EventNIDs)]
			snapshots = append(snapshots, snapshot)
		}
		if err = snapshotrows.Close(); err != nil {
			return fmt.Errorf("snapshots.Close: %w", err)
		}
		// fill in bad create snapshots
		for _, s := range badCreateSnapshots {
			var createEventNID types.EventNID
			err = tx.QueryRow(
				`SELECT event_nid FROM roomserver_events WHERE state_snapshot_nid = $1 AND event_type_nid = 1`, s.StateSnapshotNID,
			).Scan(&createEventNID)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return fmt.Errorf("cannot xref null state block with snapshot %d: %s", s.StateSnapshotNID, err)
			}
			if createEventNID == 0 {
				return fmt.Errorf("cannot xref null state block with snapshot %d, no create event", s.StateSnapshotNID)
			}
			s.EventNIDs = append(s.EventNIDs, createEventNID)
			snapshots = append(snapshots, s)
		}

		newsnapshots := map[stateSnapshotData]types.StateBlockNIDs{}

		for _, snapshot := range snapshots {
			var eventsarray pq.Int64Array
			for _, e := range snapshot.EventNIDs {
				eventsarray = append(eventsarray, int64(e))
			}

			var blocknid types.StateBlockNID
			err = tx.QueryRow(`
				INSERT INTO roomserver_state_block (state_block_hash, event_nids)
					VALUES ($1, $2)
					ON CONFLICT (state_block_hash) DO UPDATE SET event_nids=$2
					RETURNING state_block_nid
			`, snapshot.EventNIDs.Hash(), eventsarray).Scan(&blocknid)
			if err != nil {
				return fmt.Errorf("tx.QueryRow.Scan (insert new block with %d events): %w", len(eventsarray), err)
			}
			index := stateSnapshotData{snapshot.StateSnapshotNID, snapshot.RoomNID}
			newsnapshots[index] = append(newsnapshots[index], blocknid)
		}
		for snapshotdata, newblocks := range newsnapshots {
			var newblocksarray pq.Int64Array
			for _, b := range newblocks {
				newblocksarray = append(newblocksarray, int64(b))
			}

			var newNID types.StateSnapshotNID
			err = tx.QueryRow(`
					INSERT INTO roomserver_state_snapshots (state_snapshot_hash, room_nid, state_block_nids)
						VALUES ($1, $2, $3)
						ON CONFLICT (state_snapshot_hash) DO UPDATE SET room_nid=$2
						RETURNING state_snapshot_nid
				`, newblocks.Hash(), snapshotdata.RoomNID, newblocksarray).Scan(&newNID)
			if err != nil {
				return fmt.Errorf("tx.QueryRow.Scan (insert new snapshot): %w", err)
			}

			if _, err = tx.Exec(`UPDATE roomserver_events SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2 AND state_snapshot_nid<$3`, newNID, snapshotdata.StateSnapshotNID, maxsnapshotid); err != nil {
				return fmt.Errorf("tx.Exec (update events): %w", err)
			}

			if _, err = tx.Exec(`UPDATE roomserver_rooms SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2 AND state_snapshot_nid<$3`, newNID, snapshotdata.StateSnapshotNID, maxsnapshotid); err != nil {
				return fmt.Errorf("tx.Exec (update rooms): %w", err)
			}
		}
	}

	// By this point we should have no more state_snapshot_nids below maxsnapshotid in either roomserver_rooms or roomserver_events
	// If we do, this is a problem if Dendrite tries to load the snapshot as it will not exist
	// in roomserver_state_snapshots
	var count int64

	if err = tx.QueryRow(`SELECT COUNT(*) FROM roomserver_events WHERE state_snapshot_nid < $1 AND state_snapshot_nid != 0`, maxsnapshotid).Scan(&count); err != nil {
		return fmt.Errorf("assertion query failed: %s", err)
	}
	if count > 0 {
		var res sql.Result
		var c int64
		res, err = tx.Exec(`UPDATE roomserver_events SET state_snapshot_nid = 0 WHERE state_snapshot_nid < $1 AND state_snapshot_nid != 0`, maxsnapshotid)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("failed to reset invalid state snapshots: %w", err)
		}
		if c, err = res.RowsAffected(); err != nil {
			return fmt.Errorf("failed to get row count for invalid state snapshots updated: %w", err)
		} else if c != count {
			return fmt.Errorf("expected to reset %d event(s) but only updated %d event(s)", count, c)
		}
	}
	if err = tx.QueryRow(`SELECT COUNT(*) FROM roomserver_rooms WHERE state_snapshot_nid < $1 AND state_snapshot_nid != 0`, maxsnapshotid).Scan(&count); err != nil {
		return fmt.Errorf("assertion query failed: %s", err)
	}
	if count > 0 {
		var debugRoomID string
		var debugSnapNID, debugLastEventNID int64
		err = tx.QueryRow(
			`SELECT room_id, state_snapshot_nid, last_event_sent_nid FROM roomserver_rooms WHERE state_snapshot_nid < $1 AND state_snapshot_nid != 0`, maxsnapshotid,
		).Scan(&debugRoomID, &debugSnapNID, &debugLastEventNID)
		if err != nil {
			logrus.Errorf("cannot extract debug info: %v", err)
		} else {
			logrus.Errorf(
				"Affected row: room_id=%v snapshot=%v last_sent=%v",
				debugRoomID, debugSnapNID, debugLastEventNID,
			)
			logrus.Errorf("To fix this manually, run this query first then retry the migration: "+
				"UPDATE roomserver_rooms SET state_snapshot_nid=0 WHERE room_id='%v'", debugRoomID)
			logrus.Errorf("Running this UPDATE will cause the room in question to become unavailable on this server. Leave and re-join the room afterwards.")
		}
		return fmt.Errorf("%d rooms exist in roomserver_rooms which have not been converted to a new state_snapshot_nid; this is a bug, please report", count)
	}

	if _, err = tx.Exec(`
		DROP TABLE _roomserver_state_snapshots;
		DROP SEQUENCE roomserver_state_snapshot_nid_seq;
	`); err != nil {
		return fmt.Errorf("tx.Exec (delete old snapshot table): %w", err)
	}
	if _, err = tx.Exec(`
		DROP TABLE _roomserver_state_block;
		DROP SEQUENCE roomserver_state_block_nid_seq;
	`); err != nil {
		return fmt.Errorf("tx.Exec (delete old block table): %w", err)
	}

	return nil
}

func DownStateBlocksRefactor(tx *sql.Tx) error {
	panic("Downgrading state storage is not supported")
}
