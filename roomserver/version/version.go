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

package version

import (
	"errors"

	"github.com/matrix-org/gomatrixserverlib"
)

const (
	RoomVersionV1 gomatrixserverlib.RoomVersion = "1"
	RoomVersionV2 gomatrixserverlib.RoomVersion = "2"
	RoomVersionV3 gomatrixserverlib.RoomVersion = "3"
	RoomVersionV4 gomatrixserverlib.RoomVersion = "4"
	RoomVersionV5 gomatrixserverlib.RoomVersion = "5"
)

type RoomVersionDescription struct {
	Supported bool
	Stable    bool
}

var roomVersions = map[gomatrixserverlib.RoomVersion]RoomVersionDescription{
	RoomVersionV1: RoomVersionDescription{
		Supported: true,
		Stable:    true,
	},
	RoomVersionV2: RoomVersionDescription{
		Supported: true,
		Stable:    true,
	},
	RoomVersionV3: RoomVersionDescription{
		Supported: true,
		Stable:    false,
	},
	RoomVersionV4: RoomVersionDescription{
		Supported: true,
		Stable:    false,
	},
	RoomVersionV5: RoomVersionDescription{
		Supported: false,
		Stable:    false,
	},
}

func GetDefaultRoomVersion() gomatrixserverlib.RoomVersion {
	return RoomVersionV2
}

func GetRoomVersions() map[gomatrixserverlib.RoomVersion]RoomVersionDescription {
	return roomVersions
}

func GetSupportedRoomVersions() map[gomatrixserverlib.RoomVersion]RoomVersionDescription {
	versions := make(map[gomatrixserverlib.RoomVersion]RoomVersionDescription)
	for id, version := range GetRoomVersions() {
		if version.Supported {
			versions[id] = version
		}
	}
	return versions
}

func GetSupportedRoomVersion(id gomatrixserverlib.RoomVersion) (
	ver gomatrixserverlib.RoomVersion,
	desc RoomVersionDescription,
	err error,
) {
	if version, ok := roomVersions[id]; ok {
		ver = id
		desc = version
	}
	if !desc.Supported {
		err = errors.New("unsupported room version")
	}
	return
}

func GetSupportedRoomVersionFromString(version string) (
	ver gomatrixserverlib.RoomVersion,
	desc RoomVersionDescription,
	err error,
) {
	return GetSupportedRoomVersion(gomatrixserverlib.RoomVersion(version))
}
