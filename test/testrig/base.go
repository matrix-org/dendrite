// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package testrig

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/test"
)

func CreateConfig(t *testing.T, dbType test.DBType) (*config.Dendrite, *process.ProcessContext, func()) {
	var cfg config.Dendrite
	cfg.Defaults(config.DefaultOpts{
		Generate:       false,
		SingleDatabase: true,
	})
	cfg.Global.JetStream.InMemory = true
	cfg.FederationAPI.KeyPerspectives = nil
	ctx := process.NewProcessContext()
	switch dbType {
	case test.DBTypePostgres:
		cfg.Global.Defaults(config.DefaultOpts{ // autogen a signing key
			Generate:       true,
			SingleDatabase: true,
		})
		cfg.MediaAPI.Defaults(config.DefaultOpts{ // autogen a media path
			Generate:       true,
			SingleDatabase: true,
		})
		cfg.SyncAPI.Fulltext.Defaults(config.DefaultOpts{ // use in memory fts
			Generate:       true,
			SingleDatabase: true,
		})
		cfg.Global.ServerName = "test"
		// use a distinct prefix else concurrent postgres/sqlite runs will clash since NATS will use
		// the file system event with InMemory=true :(
		cfg.Global.JetStream.TopicPrefix = fmt.Sprintf("Test_%d_", dbType)
		cfg.SyncAPI.Fulltext.InMemory = true

		connStr, closeDb := test.PrepareDBConnectionString(t, dbType)
		cfg.Global.DatabaseOptions = config.DatabaseOptions{
			ConnectionString:       config.DataSource(connStr),
			MaxOpenConnections:     10,
			MaxIdleConnections:     2,
			ConnMaxLifetimeSeconds: 60,
		}
		return &cfg, ctx, func() {
			ctx.ShutdownDendrite()
			ctx.WaitForShutdown()
			closeDb()
		}
	case test.DBTypeSQLite:
		cfg.Defaults(config.DefaultOpts{
			Generate:       true,
			SingleDatabase: false,
		})
		cfg.Global.ServerName = "test"
		cfg.SyncAPI.Fulltext.Enabled = true
		cfg.SyncAPI.Fulltext.InMemory = true
		// use a distinct prefix else concurrent postgres/sqlite runs will clash since NATS will use
		// the file system event with InMemory=true :(
		cfg.Global.JetStream.TopicPrefix = fmt.Sprintf("Test_%d_", dbType)

		// Use a temp dir provided by go for tests, this will be cleanup by a call to t.CleanUp()
		tempDir := t.TempDir()
		cfg.FederationAPI.Database.ConnectionString = config.DataSource(filepath.Join("file://", tempDir, "federationapi.db"))
		cfg.KeyServer.Database.ConnectionString = config.DataSource(filepath.Join("file://", tempDir, "keyserver.db"))
		cfg.MSCs.Database.ConnectionString = config.DataSource(filepath.Join("file://", tempDir, "mscs.db"))
		cfg.MediaAPI.Database.ConnectionString = config.DataSource(filepath.Join("file://", tempDir, "mediaapi.db"))
		cfg.RoomServer.Database.ConnectionString = config.DataSource(filepath.Join("file://", tempDir, "roomserver.db"))
		cfg.SyncAPI.Database.ConnectionString = config.DataSource(filepath.Join("file://", tempDir, "syncapi.db"))
		cfg.UserAPI.AccountDatabase.ConnectionString = config.DataSource(filepath.Join("file://", tempDir, "userapi.db"))
		cfg.RelayAPI.Database.ConnectionString = config.DataSource(filepath.Join("file://", tempDir, "relayapi.db"))

		return &cfg, ctx, func() {
			ctx.ShutdownDendrite()
			ctx.WaitForShutdown()
			t.Cleanup(func() {}) // removes t.TempDir, where all database files are created
		}
	default:
		t.Fatalf("unknown db type: %v", dbType)
	}
	return &config.Dendrite{}, nil, func() {}
}
