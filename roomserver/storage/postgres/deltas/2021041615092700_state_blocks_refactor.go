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

func UpStateBlocksRefactor(tx *sql.Tx) error {
	logrus.Warn("Performing state storage upgrade. Please wait, this may take some time!")
	defer logrus.Warn("State storage upgrade complete")

	var snapshotcount int
	var maxsnapshotid int
	var maxblockid int
	if err := tx.QueryRow(`SELECT COUNT(DISTINCT state_snapshot_nid) FROM roomserver_state_snapshots;`).Scan(&snapshotcount); err != nil {
		return fmt.Errorf("tx.QueryRow.Scan (count snapshots): %w", err)
	}
	if err := tx.QueryRow(`SELECT MAX(state_snapshot_nid) FROM roomserver_state_snapshots;`).Scan(&maxsnapshotid); err != nil {
		return fmt.Errorf("tx.QueryRow.Scan (count snapshots): %w", err)
	}
	if err := tx.QueryRow(`SELECT MAX(state_block_nid) FROM roomserver_state_block;`).Scan(&maxblockid); err != nil {
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
	createblock, err := tx.Query(`
		DROP SEQUENCE IF EXISTS roomserver_state_block_nid_seq;
		CREATE SEQUENCE roomserver_state_block_nid_seq START WITH $1;

		CREATE TABLE IF NOT EXISTS roomserver_state_block (
			state_block_nid bigint PRIMARY KEY DEFAULT nextval('roomserver_state_block_nid_seq'),
			state_block_hash BYTEA UNIQUE,
			event_nids bigint[] NOT NULL
		);
	`, maxblockid)
	if err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	if err = createblock.Close(); err != nil {
		return fmt.Errorf("snapshots.Close: %w", err)
	}
	createsnapshot, err := tx.Query(`
		DROP SEQUENCE IF EXISTS roomserver_state_snapshot_nid_seq;
		CREATE SEQUENCE roomserver_state_snapshot_nid_seq START WITH $1;

		CREATE TABLE IF NOT EXISTS roomserver_state_snapshots (
			state_snapshot_nid bigint PRIMARY KEY DEFAULT nextval('roomserver_state_snapshot_nid_seq'),
			state_snapshot_hash BYTEA UNIQUE,
			room_nid bigint NOT NULL,
			state_block_nids bigint[] NOT NULL
		);
	`, maxsnapshotid)
	if err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	if err = createsnapshot.Close(); err != nil {
		return fmt.Errorf("snapshots.Close: %w", err)
	}
	logrus.Warn("New tables created...")

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
					JOIN _roomserver_state_block ON _roomserver_state_block.state_block_nid = ANY (_roomserver_state_snapshots.state_block_nids)
				WHERE
					_roomserver_state_snapshots.state_snapshot_nid = ANY ( SELECT DISTINCT
							_roomserver_state_snapshots.state_snapshot_nid
						FROM
							_roomserver_state_snapshots
						LIMIT $1 OFFSET $2)) AS _roomserver_state_block
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

		for snapshotrows.Next() {
			var snapshot stateBlockData
			var eventsarray pq.Int64Array
			if err = snapshotrows.Scan(&snapshot.StateSnapshotNID, &snapshot.RoomNID, &snapshot.StateBlockNID, &eventsarray); err != nil {
				return fmt.Errorf("rows.Scan: %w", err)
			}
			for _, e := range eventsarray {
				snapshot.EventNIDs = append(snapshot.EventNIDs, types.EventNID(e))
			}
			snapshot.EventNIDs = snapshot.EventNIDs[:util.SortAndUnique(snapshot.EventNIDs)]
			snapshots = append(snapshots, snapshot)
		}

		if err = snapshotrows.Close(); err != nil {
			return fmt.Errorf("snapshots.Close: %w", err)
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

	if _, err = tx.Exec(`DROP TABLE _roomserver_state_snapshots;`); err != nil {
		return fmt.Errorf("tx.Exec (delete old snapshot table): %w", err)
	}
	if _, err = tx.Exec(`DROP TABLE _roomserver_state_block;`); err != nil {
		return fmt.Errorf("tx.Exec (delete old block table): %w", err)
	}

	return nil
}

func DownStateBlocksRefactor(tx *sql.Tx) error {
	panic("Downgrading state storage is not supported")
}
