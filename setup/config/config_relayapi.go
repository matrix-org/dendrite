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

package config

type RelayAPI struct {
	Matrix *Global `yaml:"-"`

	InternalAPI InternalAPIOptions `yaml:"internal_api,omitempty"`
	ExternalAPI ExternalAPIOptions `yaml:"external_api,omitempty"`

	// The database stores information used by the relay queue to
	// forward transactions to remote servers.
	Database DatabaseOptions `yaml:"database,omitempty"`
}

func (c *RelayAPI) Defaults(opts DefaultOpts) {
	if !opts.Monolithic {
		c.InternalAPI.Listen = "http://localhost:7775"
		c.InternalAPI.Connect = "http://localhost:7775"
		c.ExternalAPI.Listen = "http://[::]:8075"
		c.Database.Defaults(10)
	}
	if opts.Generate {
		if !opts.Monolithic {
			c.Database.ConnectionString = "file:relayapi.db"
		}
	}
}

func (c *RelayAPI) Verify(configErrs *ConfigErrors, isMonolith bool) {
	if isMonolith { // polylith required configs below
		return
	}
	if c.Matrix.DatabaseOptions.ConnectionString == "" {
		checkNotEmpty(configErrs, "relay_api.database.connection_string", string(c.Database.ConnectionString))
	}
	checkURL(configErrs, "relay_api.external_api.listen", string(c.ExternalAPI.Listen))
	checkURL(configErrs, "relay_api.internal_api.listen", string(c.InternalAPI.Listen))
	checkURL(configErrs, "relay_api.internal_api.connect", string(c.InternalAPI.Connect))
}
