// Copyright 2017 Jan Christian Grünhage
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
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

// const filterSchema = `
// -- Stores data about filters
// CREATE TABLE IF NOT EXISTS syncapi_filter (
// 	-- The filter
// 	filter TEXT NOT NULL,
// 	-- The ID
// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
// 	-- The localpart of the Matrix user ID associated to this filter
// 	localpart TEXT NOT NULL,

// 	UNIQUE (id, localpart)
// );

// CREATE INDEX IF NOT EXISTS syncapi_filter_localpart ON syncapi_filter(localpart);
// `

type filterCosmos struct {
	ID        int64  `json:"id"`
	Filter    []byte `json:"filter"`
	Localpart string `json:"localpart"`
}

type filterCosmosData struct {
	cosmosdbapi.CosmosDocument
	Filter filterCosmos `json:"mx_syncapi_filter"`
}

// const selectFilterSQL = "" +
// 	"SELECT filter FROM syncapi_filter WHERE localpart = $1 AND id = $2"

// "SELECT id FROM syncapi_filter WHERE localpart = $1 AND filter = $2"
const selectFilterIDByContentSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_filter.localpart = @x2 " +
	"and c.mx_syncapi_filter.filter = @x3 "

// const insertFilterSQL = "" +
// 	"INSERT INTO syncapi_filter (filter, localpart) VALUES ($1, $2)"

type filterStatements struct {
	db *SyncServerDatasource
	// selectFilterStmt            *sql.Stmt
	selectFilterIDByContentStmt string
	// insertFilterStmt            *sql.Stmt
	tableName string
}

func (s *filterStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *filterStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getFilter(s *filterStatements, ctx context.Context, pk string, docId string) (*filterCosmosData, error) {
	response := filterCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, nil
	}

	return &response, err
}

func NewCosmosDBFilterTable(db *SyncServerDatasource) (tables.Filter, error) {
	s := &filterStatements{
		db: db,
	}
	s.selectFilterIDByContentStmt = selectFilterIDByContentSQL
	s.tableName = "filters"
	return s, nil
}

func (s *filterStatements) SelectFilter(
	ctx context.Context, localpart string, filterID string,
) (*gomatrixserverlib.Filter, error) {

	// "SELECT filter FROM syncapi_filter WHERE localpart = $1 AND id = $2"

	// Retrieve filter from database (stored as canonical JSON)
	var filterData []byte
	// err := s.selectFilterStmt.QueryRowContext(ctx, localpart, filterID).Scan(&filterData)

	// 	UNIQUE (id, localpart)
	docId := fmt.Sprintf("%s_%s", localpart, filterID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	var response, err = getFilter(s, ctx, s.getPartitionKey(), cosmosDocId)

	if err != nil {
		return nil, err
	}

	// Unmarshal JSON into Filter struct
	filter := gomatrixserverlib.DefaultFilter()
	if response != nil {
		filterData = response.Filter.Filter
		if err = json.Unmarshal(filterData, &filter); err != nil {
			return nil, err
		}
	}
	return &filter, nil
}

func (s *filterStatements) InsertFilter(
	ctx context.Context, filter *gomatrixserverlib.Filter, localpart string,
) (filterID string, err error) {

	// "INSERT INTO syncapi_filter (filter, localpart) VALUES ($1, $2)"

	var existingFilterID string

	// Serialise json
	filterJSON, err := json.Marshal(filter)
	if err != nil {
		return "", err
	}
	// Remove whitespaces and sort JSON data
	// needed to prevent from inserting the same filter multiple times
	filterJSON, err = gomatrixserverlib.CanonicalJSON(filterJSON)
	if err != nil {
		return "", err
	}

	// Check if filter already exists in the database using its localpart and content
	//
	// This can result in a race condition when two clients try to insert the
	// same filter and localpart at the same time, however this is not a
	// problem as both calls will result in the same filterID
	// err = s.selectFilterIDByContentStmt.QueryRowContext(ctx,
	// 	localpart, filterJSON).Scan(&existingFilterID)

	// TODO: See if we can avoid the search by Content []byte
	// "SELECT id FROM syncapi_filter WHERE localpart = $1 AND filter = $2"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": localpart,
		"@x3": filterJSON,
	}

	var rows []filterCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectFilterIDByContentStmt, params, &rows)

	if err != nil && err != cosmosdbutil.ErrNoRows {
		return "", err
	}

	if len(rows) > 0 {
		existingFilterID = fmt.Sprintf("%d", rows[0].Filter.ID)
	}
	// If it does, return the existing ID
	if existingFilterID != "" {
		return existingFilterID, nil
	}

	// Otherwise insert the filter and return the new ID
	// res, err := s.insertFilterStmt.ExecContext(ctx, filterJSON, localpart)

	// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
	seqID, seqErr := GetNextFilterID(s, ctx)
	if seqErr != nil {
		return "", seqErr
	}

	data := filterCosmos{
		ID:        seqID,
		Localpart: localpart,
		Filter:    filterJSON,
	}

	// 	UNIQUE (id, localpart)
	docId := fmt.Sprintf("%s_%d", localpart, seqID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	var dbData = filterCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		Filter:         data,
	}

	var optionsCreate = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		optionsCreate)

	if err != nil {
		return "", err
	}
	rowid := seqID
	filterID = fmt.Sprintf("%d", rowid)
	return
}
