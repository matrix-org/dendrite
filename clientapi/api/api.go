// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package api

import "github.com/matrix-org/gomatrixserverlib"

// ExtraPublicRoomsProvider provides a way to inject extra published rooms into /publicRooms requests.
type ExtraPublicRoomsProvider interface {
	// Rooms returns the extra rooms. This is called on-demand by clients, so cache appropriately.
	Rooms() []gomatrixserverlib.PublicRoom
}
