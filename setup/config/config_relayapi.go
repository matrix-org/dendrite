// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package config

type RelayAPI struct {
	Matrix *Global `yaml:"-"`

	// The database stores information used by the relay queue to
	// forward transactions to remote servers.
	Database DatabaseOptions `yaml:"database,omitempty"`
}

func (c *RelayAPI) Defaults(opts DefaultOpts) {
	if opts.Generate {
		if !opts.SingleDatabase {
			c.Database.ConnectionString = "file:relayapi.db"
		}
	}
}

func (c *RelayAPI) Verify(configErrs *ConfigErrors) {
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "relay_api.database.connection_string", string(c.Database.ConnectionString))
	}
}
