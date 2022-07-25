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
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// nolint:gocyclo
func UpStateBlocksRefactor(ctx context.Context, tx *sql.Tx) error {
	logrus.Warn("Performing state storage upgrade. Please wait, this may take some time!")
	defer logrus.Warn("State storage upgrade complete")

	var maxsnapshotid int
	var maxblockid int
	if err := tx.QueryRowContext(ctx, `SELECT IFNULL(MAX(state_snapshot_nid),0) FROM roomserver_state_snapshots;`).Scan(&maxsnapshotid); err != nil {
		return fmt.Errorf("tx.QueryRowContext.Scan (count snapshots): %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT IFNULL(MAX(state_block_nid),0) FROM roomserver_state_block;`).Scan(&maxblockid); err != nil {
		return fmt.Errorf("tx.QueryRowContext.Scan (count snapshots): %w", err)
	}
	maxsnapshotid++
	maxblockid++
	oldMaxSnapshotID := maxsnapshotid

	if _, err := tx.ExecContext(ctx, `ALTER TABLE roomserver_state_block RENAME TO _roomserver_state_block;`); err != nil {
		return fmt.Errorf("tx.ExecContext: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE roomserver_state_snapshots RENAME TO _roomserver_state_snapshots;`); err != nil {
		return fmt.Errorf("tx.ExecContext: %w", err)
	}
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS roomserver_state_block (
			state_block_nid INTEGER PRIMARY KEY AUTOINCREMENT,
			state_block_hash BLOB UNIQUE,
			event_nids TEXT NOT NULL DEFAULT '[]'
		);
	`)
	if err != nil {
		return fmt.Errorf("tx.ExecContext: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS roomserver_state_snapshots (
			state_snapshot_nid INTEGER PRIMARY KEY AUTOINCREMENT,
			state_snapshot_hash BLOB UNIQUE,
			room_nid INTEGER NOT NULL,
			state_block_nids TEXT NOT NULL DEFAULT '[]'
	  	);
	`)
	if err != nil {
		return fmt.Errorf("tx.ExecContext: %w", err)
	}
	snapshotrows, err := tx.QueryContext(ctx, `SELECT state_snapshot_nid, room_nid, state_block_nids FROM _roomserver_state_snapshots;`)
	if err != nil {
		return fmt.Errorf("tx.QueryContext: %w", err)
	}
	defer internal.CloseAndLogIfError(context.TODO(), snapshotrows, "rows.close() failed")
	for snapshotrows.Next() {
		var snapshot types.StateSnapshotNID
		var room types.RoomNID
		var jsonblocks string
		var blocks []types.StateBlockNID
		if err = snapshotrows.Scan(&snapshot, &room, &jsonblocks); err != nil {
			return fmt.Errorf("rows.Scan: %w", err)
		}
		if err = json.Unmarshal([]byte(jsonblocks), &blocks); err != nil {
			return fmt.Errorf("json.Unmarshal: %w", err)
		}

		var newblocks types.StateBlockNIDs
		if len(blocks) == 0 {
			// some m.room.create events have a state snapshot but no state blocks at all which makes
			// sense as there is no state before creation. The correct form should be to give the event
			// in question a state snapshot NID of 0 to indicate 'no snapshot'.
			// If we don't do this, we'll fail the assertions later on which try to ensure we didn't forget
			// any snapshots.
			_, err = tx.ExecContext(ctx,
				`UPDATE roomserver_events SET state_snapshot_nid = 0 WHERE event_type_nid = $1 AND event_state_key_nid = $2 AND state_snapshot_nid = $3`,
				types.MRoomCreateNID, types.EmptyStateKeyNID, snapshot,
			)
			if err != nil {
				return fmt.Errorf("resetting create events snapshots to 0 errored: %s", err)
			}
		}
		for _, block := range blocks {
			if err = func() error {
				blockrows, berr := tx.QueryContext(ctx, `SELECT event_nid FROM _roomserver_state_block WHERE state_block_nid = $1`, block)
				if berr != nil {
					return fmt.Errorf("tx.QueryContext (event nids from old block): %w", berr)
				}
				defer internal.CloseAndLogIfError(context.TODO(), blockrows, "rows.close() failed")
				events := types.EventNIDs{}
				for blockrows.Next() {
					var event types.EventNID
					if err = blockrows.Scan(&event); err != nil {
						return fmt.Errorf("rows.Scan: %w", err)
					}
					events = append(events, event)
				}
				events = events[:util.SortAndUnique(events)]
				eventjson, eerr := json.Marshal(events)
				if eerr != nil {
					return fmt.Errorf("json.Marshal: %w", eerr)
				}

				var blocknid types.StateBlockNID
				err = tx.QueryRowContext(ctx, `
					INSERT INTO roomserver_state_block (state_block_nid, state_block_hash, event_nids)
						VALUES ($1, $2, $3)
						ON CONFLICT (state_block_hash) DO UPDATE SET event_nids=$3
						RETURNING state_block_nid
				`, maxblockid, events.Hash(), eventjson).Scan(&blocknid)
				if err != nil {
					return fmt.Errorf("tx.QueryRowContext.Scan (insert new block): %w", err)
				}
				maxblockid++
				newblocks = append(newblocks, blocknid)
				return nil
			}(); err != nil {
				return err
			}

			newblocksjson, jerr := json.Marshal(newblocks)
			if jerr != nil {
				return fmt.Errorf("json.Marshal (new blocks): %w", jerr)
			}

			var newsnapshot types.StateSnapshotNID
			err = tx.QueryRowContext(ctx, `
				INSERT INTO roomserver_state_snapshots (state_snapshot_nid, state_snapshot_hash, room_nid, state_block_nids)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT (state_snapshot_hash) DO UPDATE SET room_nid=$3
					RETURNING state_snapshot_nid
			`, maxsnapshotid, newblocks.Hash(), room, newblocksjson).Scan(&newsnapshot)
			if err != nil {
				return fmt.Errorf("tx.QueryRowContext.Scan (insert new snapshot): %w", err)
			}
			maxsnapshotid++
			_, err = tx.ExecContext(ctx, `UPDATE roomserver_events SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2 AND state_snapshot_nid<$3`, newsnapshot, snapshot, maxsnapshotid)
			if err != nil {
				return fmt.Errorf("tx.ExecContext (update events): %w", err)
			}
			if _, err = tx.ExecContext(ctx, `UPDATE roomserver_rooms SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2 AND state_snapshot_nid<$3`, newsnapshot, snapshot, maxsnapshotid); err != nil {
				return fmt.Errorf("tx.ExecContext (update rooms): %w", err)
			}
		}
	}

	// By this point we should have no more state_snapshot_nids below oldMaxSnapshotID in either roomserver_rooms or roomserver_events
	// If we do, this is a problem if Dendrite tries to load the snapshot as it will not exist
	// in roomserver_state_snapshots
	var count int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM roomserver_events WHERE state_snapshot_nid < $1 AND state_snapshot_nid != 0`, oldMaxSnapshotID).Scan(&count); err != nil {
		return fmt.Errorf("assertion query failed: %s", err)
	}
	if count > 0 {
		var res sql.Result
		var c int64
		res, err = tx.ExecContext(ctx, `UPDATE roomserver_events SET state_snapshot_nid = 0 WHERE state_snapshot_nid < $1 AND state_snapshot_nid != 0`, oldMaxSnapshotID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("failed to reset invalid state snapshots: %w", err)
		}
		if c, err = res.RowsAffected(); err != nil {
			return fmt.Errorf("failed to get row count for invalid state snapshots updated: %w", err)
		} else if c != count {
			return fmt.Errorf("expected to reset %d event(s) but only updated %d event(s)", count, c)
		}
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM roomserver_rooms WHERE state_snapshot_nid < $1 AND state_snapshot_nid != 0`, oldMaxSnapshotID).Scan(&count); err != nil {
		return fmt.Errorf("assertion query failed: %s", err)
	}
	if count > 0 {
		return fmt.Errorf("%d rooms exist in roomserver_rooms which have not been converted to a new state_snapshot_nid; this is a bug, please report", count)
	}

	if _, err = tx.ExecContext(ctx, `DROP TABLE _roomserver_state_snapshots;`); err != nil {
		return fmt.Errorf("tx.Exec (delete old snapshot table): %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DROP TABLE _roomserver_state_block;`); err != nil {
		return fmt.Errorf("tx.Exec (delete old block table): %w", err)
	}

	return nil
}

func DownStateBlocksRefactor(ctx context.Context, tx *sql.Tx) error {
	panic("Downgrading state storage is not supported")
}
