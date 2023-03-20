// Copyright 2023 The Matrix.org Foundation C.I.C.
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

package sqlutil

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/setup/config"
)

type Connections struct {
	db           *sql.DB
	writer       Writer
	globalConfig config.DatabaseOptions
}

func NewConnectionManager(globalConfig config.DatabaseOptions) Connections {
	return Connections{
		globalConfig: globalConfig,
	}
}

func (c *Connections) Connection(dbProperties *config.DatabaseOptions) (*sql.DB, Writer, error) {
	writer := NewDummyWriter()
	if dbProperties.ConnectionString.IsSQLite() {
		writer = NewExclusiveWriter()
	}
	var err error
	if dbProperties.ConnectionString == "" {
		// if no connectionString was provided, try the global one
		dbProperties = &c.globalConfig
	}
	if dbProperties.ConnectionString != "" || c.db == nil {
		// Open a new database connection using the supplied config.
		c.db, err = Open(dbProperties, writer)
		if err != nil {
			return nil, nil, err
		}
		c.writer = writer
		return c.db, c.writer, nil
	}
	if c.db != nil && c.writer != nil {
		// Ignore the supplied config and return the global pool and
		// writer.
		return c.db, c.writer, nil
	}
	return nil, nil, fmt.Errorf("no database connections configured")
}
