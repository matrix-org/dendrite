// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
)

func CreateBaseDendrite(t *testing.T, dbType DBType) (*base.BaseDendrite, func()) {
	var cfg config.Dendrite
	cfg.Defaults(false)

	switch dbType {
	case DBTypePostgres:
		cfg.Global.Defaults(true)   // autogen a signing key
		cfg.MediaAPI.Defaults(true) // autogen a media path
		connStr, close := PrepareDBConnectionString(t, dbType)
		cfg.Global.DatabaseOptions = config.DatabaseOptions{
			ConnectionString:       config.DataSource(connStr),
			MaxOpenConnections:     10,
			MaxIdleConnections:     2,
			ConnMaxLifetimeSeconds: 60,
		}
		return base.NewBaseDendrite(&cfg, "Test", base.DisableMetrics), close
	case DBTypeSQLite:
		cfg.Defaults(true) // sets a sqlite db per component
		return base.NewBaseDendrite(&cfg, "Test", base.DisableMetrics), func() {
			// cleanup db files. This risks getting out of sync as we add more database strings :(
			dbFiles := []config.DataSource{
				cfg.AppServiceAPI.Database.ConnectionString,
				cfg.FederationAPI.Database.ConnectionString,
				cfg.KeyServer.Database.ConnectionString,
				cfg.MSCs.Database.ConnectionString,
				cfg.MediaAPI.Database.ConnectionString,
				cfg.RoomServer.Database.ConnectionString,
				cfg.SyncAPI.Database.ConnectionString,
				cfg.UserAPI.AccountDatabase.ConnectionString,
			}
			for _, fileURI := range dbFiles {
				path := strings.TrimPrefix(string(fileURI), "file:")
				err := os.Remove(path)
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("failed to cleanup sqlite db '%s': %s", fileURI, err)
				}
			}
		}
	default:
		t.Fatalf("unknown db type: %v", dbType)
	}
	return nil, nil
}
