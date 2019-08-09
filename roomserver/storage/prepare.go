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

// a statementList is a list of SQL statements to prepare and a pointer to where to store the resulting prepared statement.
type statementList []struct {
	statement **sql.Stmt
	sql       string
}

// prepare the SQL for each statement in the list and assign the result to the prepared statement.
func (s statementList) prepare(db *sql.DB) (err error) {
	for _, statement := range s {
		if *statement.statement, err = db.Prepare(statement.sql); err != nil {
			return
		}
	}
	return
}
