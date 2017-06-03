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
	// The base path to where the media files will be stored. May be relative or absolute.
	BasePath types.Path `yaml:"base_path"`
	// The absolute base path to where media files will be stored.
	AbsBasePath types.Path `yaml:"-"`
	// The maximum file size in bytes that is allowed to be stored on this server.
	// Note: if MaxFileSizeBytes is set to 0, the size is unlimited.
	// Note: if max_file_size_bytes is not set, it will default to 10485760 (10MB)
	MaxFileSizeBytes *types.FileSizeBytes `yaml:"max_file_size_bytes,omitempty"`
	// The postgres connection config for connecting to the database e.g a postgres:// URI
	DataSource string `yaml:"database"`
	// Whether to dynamically generate thumbnails on-the-fly if the requested resolution is not already generated
	DynamicThumbnails bool `yaml:"dynamic_thumbnails"`
	// A list of thumbnail sizes to be pre-generated for downloaded remote / uploaded content
	ThumbnailSizes []types.ThumbnailSize `yaml:"thumbnail_sizes"`
}
