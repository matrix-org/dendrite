// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlutil

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
)

type Connections struct {
	globalConfig        config.DatabaseOptions
	processContext      *process.ProcessContext
	existingConnections sync.Map
}

type con struct {
	db     *sql.DB
	writer Writer
}

func NewConnectionManager(processCtx *process.ProcessContext, globalConfig config.DatabaseOptions) *Connections {
	return &Connections{
		globalConfig:   globalConfig,
		processContext: processCtx,
	}
}

func (c *Connections) Connection(dbProperties *config.DatabaseOptions) (*sql.DB, Writer, error) {
	var err error
	// If no connectionString was provided, try the global one
	if dbProperties.ConnectionString == "" {
		dbProperties = &c.globalConfig
		// If we still don't have a connection string, that's a problem
		if dbProperties.ConnectionString == "" {
			return nil, nil, fmt.Errorf("no database connections configured")
		}
	}

	writer := NewDummyWriter()
	if dbProperties.ConnectionString.IsSQLite() {
		writer = NewExclusiveWriter()
	}

	existing, loaded := c.existingConnections.LoadOrStore(dbProperties.ConnectionString, &con{})
	if loaded {
		// We found an existing connection
		ex := existing.(*con)
		return ex.db, ex.writer, nil
	}

	// Open a new database connection using the supplied config.
	db, err := Open(dbProperties, writer)
	if err != nil {
		return nil, nil, err
	}
	c.existingConnections.Store(dbProperties.ConnectionString, &con{db: db, writer: writer})
	go func() {
		if c.processContext == nil {
			return
		}
		// If we have a ProcessContext, start a component and wait for
		// Dendrite to shut down to cleanly close the database connection.
		c.processContext.ComponentStarted()
		<-c.processContext.WaitForShutdown()
		_ = db.Close()
		c.processContext.ComponentFinished()
	}()
	return db, writer, nil

}
