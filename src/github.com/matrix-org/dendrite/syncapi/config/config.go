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
	"github.com/matrix-org/gomatrixserverlib"
)

// Sync contains the config information necessary to spin up a sync-server process.
type Sync struct {
	// The topic for events which are written by the room server output log.
	RoomserverOutputTopic string `yaml:"roomserver_topic"`
	// A list of URIs to consume events from. These kafka logs should be produced by a Room Server.
	KafkaConsumerURIs []string `yaml:"consumer_uris"`
	// The postgres connection config for connecting to the database e.g a postgres:// URI
	DataSource string `yaml:"database"`
	// The postgres connection config for connecting to the account database e.g a postgres:// URI
	AccountDataSource string `yaml:"account_database"`
	// The server_name of the running process e.g "localhost"
	ServerName gomatrixserverlib.ServerName `yaml:"server_name"`
}
