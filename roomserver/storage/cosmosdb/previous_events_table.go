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

package cosmosdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

// TODO: previous_reference_sha256 was NOT NULL before but it broke sytest because
// sytest sends no SHA256 sums in the prev_events references in the soft-fail tests.
// In Postgres an empty BYTEA field is not NULL so it's fine there. In SQLite it
// seems to care that it's empty and therefore hits a NOT NULL constraint on insert.
// We should really work out what the right thing to do here is.
// const previousEventSchema = `
//   CREATE TABLE IF NOT EXISTS roomserver_previous_events (
//     previous_event_id TEXT NOT NULL,
//     previous_reference_sha256 BLOB,
//     event_nids TEXT NOT NULL,
//     UNIQUE (previous_event_id, previous_reference_sha256)
//   );
// `

type PreviousEventCosmos struct {
	PreviousEventID         string `json:"previous_event_id"`
	PreviousReferenceSha256 []byte `json:"previous_reference_sha256"`
	EventNIDs               string `json:"event_nids"`
}

type PreviousEventCosmosData struct {
	Id            string              `json:"id"`
	Pk            string              `json:"_pk"`
	Tn            string              `json:"_sid"`
	Cn            string              `json:"_cn"`
	ETag          string              `json:"_etag"`
	Timestamp     int64               `json:"_ts"`
	PreviousEvent PreviousEventCosmos `json:"mx_roomserver_previous_event"`
}

// Insert an entry into the previous_events table.
// If there is already an entry indicating that an event references that previous event then
// add the event NID to the list to indicate that this event references that previous event as well.
// This should only be modified while holding a "FOR UPDATE" lock on the row in the rooms table for this room.
// The lock is necessary to avoid data races when checking whether an event is already referenced by another event.
// const insertPreviousEventSQL = `
// 	INSERT OR REPLACE INTO roomserver_previous_events
// 	  (previous_event_id, previous_reference_sha256, event_nids)
// 	  VALUES ($1, $2, $3)
// `

// const selectPreviousEventNIDsSQL = `
// 	SELECT event_nids FROM roomserver_previous_events
// 	  WHERE previous_event_id = $1 AND previous_reference_sha256 = $2
// `

// Check if the event is referenced by another event in the table.
// This should only be done while holding a "FOR UPDATE" lock on the row in the rooms table for this room.
// const selectPreviousEventExistsSQL = `
// 	SELECT 1 FROM roomserver_previous_events
// 	  WHERE previous_event_id = $1 AND previous_reference_sha256 = $2
// `

type previousEventStatements struct {
	db *Database
	// insertPreviousEventStmt       *sql.Stmt
	// selectPreviousEventNIDsStmt   *sql.Stmt
	// selectPreviousEventExistsStmt *sql.Stmt
	tableName string
}

func getPreviousEvent(s *previousEventStatements, ctx context.Context, pk string, docId string) (*PreviousEventCosmosData, error) {
	response := PreviousEventCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, cosmosdbutil.ErrNoRows
	}

	return &response, err
}

func NewCosmosDBPrevEventsTable(db *Database) (tables.PreviousEvents, error) {
	s := &previousEventStatements{
		db: db,
	}

	// return s, shared.StatementList{
	// 	{&s.insertPreviousEventStmt, insertPreviousEventSQL},
	// 	{&s.selectPreviousEventNIDsStmt, selectPreviousEventNIDsSQL},
	// 	{&s.selectPreviousEventExistsStmt, selectPreviousEventExistsSQL},
	// }.Prepare(db)
	s.tableName = "previous_events"
	return s, nil
}

func (s *previousEventStatements) InsertPreviousEvent(
	ctx context.Context,
	txn *sql.Tx,
	previousEventID string,
	previousEventReferenceSHA256 []byte,
	eventNID types.EventNID,
) error {
	eventNIDAsString := fmt.Sprintf("%d", eventNID)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	//     UNIQUE (previous_event_id, previous_reference_sha256)
	// TODO: Check value
	// docId := fmt.Sprintf("%s_%s", previousEventID, previousEventReferenceSHA256)
	docId := previousEventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	// SELECT 1 FROM roomserver_previous_events
	//   WHERE previous_event_id = $1 AND previous_reference_sha256 = $2
	existing, err := getPreviousEvent(s, ctx, pk, cosmosDocId)

	if err != nil {
		if err != cosmosdbutil.ErrNoRows {
			return fmt.Errorf("selectStmt.QueryRowContext.Scan: %w", err)
		}
	}

	var dbData PreviousEventCosmosData
	// Doesnt exist, create a new one
	if existing == nil {
		data := PreviousEventCosmos{
			EventNIDs:               "",
			PreviousEventID:         previousEventID,
			PreviousReferenceSha256: previousEventReferenceSHA256,
		}

		dbData = PreviousEventCosmosData{
			Id:            cosmosDocId,
			Tn:            s.db.cosmosConfig.TenantName,
			Cn:            dbCollectionName,
			Pk:            pk,
			Timestamp:     time.Now().Unix(),
			PreviousEvent: data,
		}
	} else {
		dbData = *existing
	}

	var nids []string
	if dbData.PreviousEvent.EventNIDs != "" {
		nids = strings.Split(dbData.PreviousEvent.EventNIDs, ",")
		for _, nid := range nids {
			if nid == eventNIDAsString {
				return nil
			}
		}
		dbData.PreviousEvent.EventNIDs = strings.Join(append(nids, eventNIDAsString), ",")
	} else {
		dbData.PreviousEvent.EventNIDs = eventNIDAsString
	}

	// INSERT OR REPLACE INTO roomserver_previous_events
	//   (previous_event_id, previous_reference_sha256, event_nids)
	//   VALUES ($1, $2, $3)

	var optionsReplace = cosmosdbapi.GetUpsertDocumentOptions(pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		optionsReplace,
	)
	return err
}

// Check if the event reference exists
// Returns sql.ErrNoRows if the event reference doesn't exist.
func (s *previousEventStatements) SelectPreviousEventExists(
	ctx context.Context, txn *sql.Tx, eventID string, eventReferenceSHA256 []byte,
) error {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	//     UNIQUE (previous_event_id, previous_reference_sha256)
	// TODO: Check value
	// docId := fmt.Sprintf("%s_%s", previousEventID, previousEventReferenceSHA256)
	docId := eventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, string(docId))
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	// SELECT 1 FROM roomserver_previous_events
	//   WHERE previous_event_id = $1 AND previous_reference_sha256 = $2
	dbData, err := getPreviousEvent(s, ctx, pk, cosmosDocId)
	if err != nil {
		return err
	}

	if dbData == nil {
		return cosmosdbutil.ErrNoRows
	}
	return nil
}
