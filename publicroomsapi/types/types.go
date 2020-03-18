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

package types

// ExternalPublicRoomsProvider provides a list of homeservers who should be queried
// periodically for a list of public rooms on their server.
type ExternalPublicRoomsProvider interface {
	// The list of homeserver domains to query. These servers will receive a request
	// via this API: https://matrix.org/docs/spec/server_server/latest#public-room-directory
	// This will be called -on demand- by clients, so cache appropriately!
	Homeservers() []string
}
