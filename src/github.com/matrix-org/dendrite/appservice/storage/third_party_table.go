// Copyright 2018 New Vector Ltd
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
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/appservice/types"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

const thirdPartySchema = `
-- Stores protocol definitions for clients to later request
CREATE TABLE IF NOT EXISTS appservice_third_party_protocol_def (
	-- The ID of the procotol
	protocol_id TEXT NOT NULL PRIMARY KEY,
	-- The JSON-encoded protocol definition
	protocol_definition TEXT NOT NULL,
	UNIQUE(protocol_id)
);

CREATE INDEX IF NOT EXISTS appservice_third_party_protocol_def_id
ON appservice_third_party_protocol_def(protocol_id);
`

const selectProtocolDefinitionSQL = "" +
	"SELECT protocol_definition FROM appservice_third_party_protocol_def " +
	"WHERE protocol_id = $1"

const selectAllProtocolDefinitionsSQL = "" +
	"SELECT protocol_id, protocol_definition FROM appservice_third_party_protocol_def"

const insertProtocolDefinitionSQL = "" +
	"INSERT INTO appservice_third_party_protocol_def(protocol_id, protocol_definition) " +
	"VALUES ($1, $2)"

const clearProtocolDefinitionsSQL = "" +
	"TRUNCATE appservice_third_party_protocol_def"

type thirdPartyStatements struct {
	selectProtocolDefinitionStmt     *sql.Stmt
	selectAllProtocolDefinitionsStmt *sql.Stmt
	insertProtocolDefinitionStmt     *sql.Stmt
	clearProtocolDefinitionsStmt     *sql.Stmt
}

func (s *thirdPartyStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(thirdPartySchema)
	if err != nil {
		return
	}

	if s.selectProtocolDefinitionStmt, err = db.Prepare(selectProtocolDefinitionSQL); err != nil {
		return
	}

	if s.selectAllProtocolDefinitionsStmt, err = db.Prepare(selectAllProtocolDefinitionsSQL); err != nil {
		return
	}

	if s.insertProtocolDefinitionStmt, err = db.Prepare(insertProtocolDefinitionSQL); err != nil {
		return
	}

	if s.clearProtocolDefinitionsStmt, err = db.Prepare(clearProtocolDefinitionsSQL); err != nil {
		return
	}

	return
}

// selectProtocolDefinition returns a single protocol definition for a given ID.
// Returns an empty string if the ID was not found.
func (s *thirdPartyStatements) selectProtocolDefinition(
	ctx context.Context,
	protocolID string,
) (protocolDefinition string, err error) {
	err = s.selectProtocolDefinitionStmt.QueryRowContext(
		ctx, protocolID,
	).Scan(&protocolDefinition)

	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	return
}

// selectAllProtocolDefinitions returns all protcol IDs and definitions in the
// database. Returns an empty map if no definitions were found.
func (s *thirdPartyStatements) selectAllProtocolDefinitions(
	ctx context.Context,
) (protocols types.ThirdPartyProtocols, err error) {
	protocolDefinitionRows, err := s.selectAllProtocolDefinitionsStmt.QueryContext(ctx)
	if err != nil && err != sql.ErrNoRows {
		return
	}
	defer func() {
		err = protocolDefinitionRows.Close()
		if err != nil {
			log.WithError(err).Fatalf("unable to close protocol definitions")
		}
	}()

	for protocolDefinitionRows.Next() {
		var protocolID, protocolDefinition string
		if err = protocolDefinitionRows.Scan(&protocolID, &protocolDefinition); err != nil {
			return nil, err
		}

		protocols[protocolID] = gomatrixserverlib.RawJSON(protocolDefinition)
	}

	return protocols, nil
}

// insertProtocolDefinition inserts a protocol ID along with its definition in
// order for clients to later retreive it from the client-server API.
func (s *thirdPartyStatements) insertProtocolDefinition(
	ctx context.Context,
	protocolID, protocolDefinition string,
) (err error) {
	_, err = s.insertProtocolDefinitionStmt.ExecContext(
		ctx,
		protocolID,
		protocolDefinition,
	)
	return
}

// clearProtocolDefinitions removes all protocol definitions from the database.
func (s *thirdPartyStatements) clearProtocolDefinitions(
	ctx context.Context,
) (err error) {
	_, err = s.clearProtocolDefinitionsStmt.ExecContext(ctx)
	return
}
