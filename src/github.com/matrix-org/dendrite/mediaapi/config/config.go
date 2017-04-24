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

// MediaAPI contains the config information necessary to spin up a mediaapi process.
type MediaAPI struct {
	// The name of the server. This is usually the domain name, e.g 'matrix.org', 'localhost'.
	ServerName string `yaml:"server_name"`
	// The base path to where media files will be stored.
	BasePath string `yaml:"base_path"`
	// The postgres connection config for connecting to the database e.g a postgres:// URI
	DataSource string `yaml:"database"`
}
