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

package config

import (
	"github.com/matrix-org/dendrite/mediaapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// MediaAPI contains the config information necessary to spin up a mediaapi process.
type MediaAPI struct {
	// The name of the server. This is usually the domain name, e.g 'matrix.org', 'localhost'.
	ServerName gomatrixserverlib.ServerName `yaml:"server_name"`
	// The absolute base path to where media files will be stored.
	AbsBasePath types.Path `yaml:"abs_base_path"`
	// The maximum file size in bytes that is allowed to be stored on this server.
	// Note that remote files larger than this can still be proxied to a client, they will just not be cached.
	// Note: if MaxFileSizeBytes is set to 0, the size is unlimited.
	MaxFileSizeBytes types.ContentLength `yaml:"max_file_size_bytes"`
	// The postgres connection config for connecting to the database e.g a postgres:// URI
	DataSource string `yaml:"database"`
}
