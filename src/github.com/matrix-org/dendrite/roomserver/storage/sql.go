// Copyright 2017 Vector Creations Ltd
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
	"database/sql"
)

type statements struct {
	eventTypeStatements
	eventStateKeyStatements
	roomStatements
	eventStatements
	eventJSONStatements
	stateSnapshotStatements
	stateBlockStatements
	previousEventStatements
	roomAliasesStatements
}

func (s *statements) prepare(db *sql.DB) error {
	var err error

	if err = s.eventTypeStatements.prepare(db); err != nil {
		return err
	}

	if err = s.eventStateKeyStatements.prepare(db); err != nil {
		return err
	}

	if err = s.roomStatements.prepare(db); err != nil {
		return err
	}

	if err = s.eventStatements.prepare(db); err != nil {
		return err
	}

	if err = s.eventJSONStatements.prepare(db); err != nil {
		return err
	}

	if err = s.stateSnapshotStatements.prepare(db); err != nil {
		return err
	}

	if err = s.stateBlockStatements.prepare(db); err != nil {
		return err
	}

	if err = s.previousEventStatements.prepare(db); err != nil {
		return err
	}

	if err = s.roomAliasesStatements.prepare(db); err != nil {
		return err
	}

	return nil
}
