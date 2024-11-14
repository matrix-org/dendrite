// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package deltas

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/element-hq/dendrite/internal"
	"github.com/lib/pq"
	"github.com/matrix-org/util"
)

func UpDropEventReferenceSHAEvents(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `ALTER TABLE roomserver_events DROP COLUMN IF EXISTS reference_sha256;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

func UpDropEventReferenceSHAPrevEvents(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "ALTER TABLE roomserver_previous_events DROP CONSTRAINT roomserver_previous_event_id_unique;")
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	_, err = tx.ExecContext(ctx, `ALTER TABLE roomserver_previous_events DROP COLUMN IF EXISTS previous_reference_sha256;`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}

	// figure out if there are duplicates
	dupeRows, err := tx.QueryContext(ctx, `SELECT previous_event_id FROM roomserver_previous_events GROUP BY previous_event_id HAVING count(previous_event_id) > 1`)
	if err != nil {
		return fmt.Errorf("failed to query duplicate event ids")
	}
	defer internal.CloseAndLogIfError(ctx, dupeRows, "failed to close rows")

	var prevEvents []string
	var prevEventID string
	for dupeRows.Next() {
		if err = dupeRows.Scan(&prevEventID); err != nil {
			return err
		}
		prevEvents = append(prevEvents, prevEventID)
	}
	if dupeRows.Err() != nil {
		return dupeRows.Err()
	}

	// if we found duplicates, check if we can combine them, e.g. they are in the same room
	for _, dupeID := range prevEvents {
		var dupeNIDsRows *sql.Rows
		dupeNIDsRows, err = tx.QueryContext(ctx, `SELECT event_nids FROM roomserver_previous_events WHERE previous_event_id = $1`, dupeID)
		if err != nil {
			return fmt.Errorf("failed to query duplicate event ids")
		}
		defer internal.CloseAndLogIfError(ctx, dupeNIDsRows, "failed to close rows")
		var dupeNIDs []int64
		for dupeNIDsRows.Next() {
			var nids pq.Int64Array
			if err = dupeNIDsRows.Scan(&nids); err != nil {
				return err
			}
			dupeNIDs = append(dupeNIDs, nids...)
		}

		if dupeNIDsRows.Err() != nil {
			return dupeNIDsRows.Err()
		}
		// dedupe NIDs
		dupeNIDs = dupeNIDs[:util.SortAndUnique(nids(dupeNIDs))]
		// now that we have all NIDs, check which room they belong to
		var roomCount int
		err = tx.QueryRowContext(ctx, `SELECT count(distinct room_nid) FROM roomserver_events WHERE event_nid = ANY($1)`, pq.Array(dupeNIDs)).Scan(&roomCount)
		if err != nil {
			return err
		}
		// if the events are from different rooms, that's bad and we can't continue
		if roomCount > 1 {
			return fmt.Errorf("detected events (%v) referenced for different rooms (%v)", dupeNIDs, roomCount)
		}
		// otherwise delete the dupes
		_, err = tx.ExecContext(ctx, "DELETE FROM roomserver_previous_events WHERE previous_event_id = $1", dupeID)
		if err != nil {
			return fmt.Errorf("unable to delete duplicates: %w", err)
		}

		// insert combined values
		_, err = tx.ExecContext(ctx, "INSERT INTO roomserver_previous_events (previous_event_id, event_nids) VALUES ($1, $2)", dupeID, pq.Array(dupeNIDs))
		if err != nil {
			return fmt.Errorf("unable to insert new event NIDs: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx, `ALTER TABLE roomserver_previous_events ADD CONSTRAINT roomserver_previous_event_id_unique UNIQUE (previous_event_id);`)
	if err != nil {
		return fmt.Errorf("failed to execute upgrade: %w", err)
	}
	return nil
}

type nids []int64

func (s nids) Len() int           { return len(s) }
func (s nids) Less(i, j int) bool { return s[i] < s[j] }
func (s nids) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
