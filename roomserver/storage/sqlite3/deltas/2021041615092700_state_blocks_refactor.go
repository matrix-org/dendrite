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
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

func LoadStateBlocksRefactor(m *sqlutil.Migrations) {
	m.AddMigration(UpStateBlocksRefactor, DownStateBlocksRefactor)
}

func UpStateBlocksRefactor(tx *sql.Tx) error {
	logrus.Warn("Performing state block refactor upgrade. Please wait, this may take some time!")

	if _, err := tx.Exec(`ALTER TABLE roomserver_state_block RENAME TO _roomserver_state_block;`); err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE roomserver_state_snapshots RENAME TO _roomserver_state_snapshots;`); err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS roomserver_state_block (
			state_block_nid INTEGER PRIMARY KEY AUTOINCREMENT,
			state_block_hash BLOB UNIQUE,
			event_nids TEXT NOT NULL DEFAULT '[]'
		);
	`)
	if err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS roomserver_state_snapshots (
			state_snapshot_nid INTEGER PRIMARY KEY AUTOINCREMENT,
			state_snapshot_hash BLOB UNIQUE,
			room_nid INTEGER NOT NULL,
			state_block_nids TEXT NOT NULL DEFAULT '[]'
	  	);
	`)
	if err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	snapshotrows, err := tx.Query(`SELECT state_snapshot_nid, room_nid, state_block_nids FROM _roomserver_state_snapshots;`)
	if err != nil {
		return fmt.Errorf("tx.Query: %w", err)
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
		for _, block := range blocks {
			if err = func() error {
				blockrows, berr := tx.Query(`SELECT event_nid FROM _roomserver_state_block WHERE state_block_nid = $1`, block)
				if berr != nil {
					return fmt.Errorf("tx.Query (event nids from old block): %w", berr)
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
				err = tx.QueryRow(`
					INSERT INTO roomserver_state_block (state_block_hash, event_nids)
						VALUES ($1, $2)
						ON CONFLICT (state_block_hash) DO UPDATE SET event_nids=$2
						RETURNING state_block_nid
				`, events.Hash(), eventjson).Scan(&blocknid)
				if err != nil {
					return fmt.Errorf("tx.QueryRow.Scan (insert new block): %w", err)
				}
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
			err = tx.QueryRow(`
				INSERT INTO roomserver_state_snapshots (state_snapshot_hash, room_nid, state_block_nids)
					VALUES ($1, $2, $3)
					ON CONFLICT (state_snapshot_hash) DO UPDATE SET room_nid=$2
					RETURNING state_snapshot_nid
			`, newblocks.Hash(), room, newblocksjson).Scan(&newsnapshot)
			if err != nil {
				return fmt.Errorf("tx.QueryRow.Scan (insert new snapshot): %w", err)
			}

			if _, err = tx.Exec(`UPDATE roomserver_events SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2`, newsnapshot, snapshot); err != nil {
				return fmt.Errorf("tx.Exec (update events): %w", err)
			}
			if _, err = tx.Exec(`UPDATE roomserver_rooms SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2`, newsnapshot, snapshot); err != nil {
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
