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

	"github.com/matrix-org/dendrite/roomserver/state"
)

type RoomVersionID int
type EventFormatID int

const (
	RoomVersionV1 RoomVersionID = iota + 1
	RoomVersionV2
	RoomVersionV3
	RoomVersionV4
	RoomVersionV5
)

const (
	EventFormatV1 EventFormatID = iota + 1 // original event ID formatting
	EventFormatV2                          // event ID is event hash
	EventFormatV3                          // event ID is URL-safe base64 event hash
)

type RoomVersionDescription struct {
	Supported                 bool
	Stable                    bool
	StateResolution           state.StateResolutionVersion
	EventFormat               EventFormatID
	EnforceSigningKeyValidity bool
}

var roomVersions = map[RoomVersionID]RoomVersionDescription{
	RoomVersionV1: RoomVersionDescription{
		Supported:                 true,
		Stable:                    true,
		StateResolution:           state.StateResolutionAlgorithmV1,
		EventFormat:               EventFormatV1,
		EnforceSigningKeyValidity: false,
	},
	RoomVersionV2: RoomVersionDescription{
		Supported:                 false,
		Stable:                    true,
		StateResolution:           state.StateResolutionAlgorithmV2,
		EventFormat:               EventFormatV1,
		EnforceSigningKeyValidity: false,
	},
	RoomVersionV3: RoomVersionDescription{
		Supported:                 false,
		Stable:                    true,
		StateResolution:           state.StateResolutionAlgorithmV2,
		EventFormat:               EventFormatV2,
		EnforceSigningKeyValidity: false,
	},
	RoomVersionV4: RoomVersionDescription{
		Supported:                 false,
		Stable:                    true,
		StateResolution:           state.StateResolutionAlgorithmV2,
		EventFormat:               EventFormatV3,
		EnforceSigningKeyValidity: false,
	},
	RoomVersionV5: RoomVersionDescription{
		Supported:                 false,
		Stable:                    true,
		StateResolution:           state.StateResolutionAlgorithmV2,
		EventFormat:               EventFormatV3,
		EnforceSigningKeyValidity: true,
	},
}

func GetDefaultRoomVersion() RoomVersionID {
	return RoomVersionV1
}

func GetRoomVersions() map[RoomVersionID]RoomVersionDescription {
	return roomVersions
}

func GetSupportedRoomVersions() map[RoomVersionID]RoomVersionDescription {
	versions := make(map[RoomVersionID]RoomVersionDescription)
	for id, version := range GetRoomVersions() {
		if version.Supported {
			versions[id] = version
		}
	}
	return versions
}

func GetSupportedRoomVersion(version RoomVersionID) (desc RoomVersionDescription, err error) {
	if version, ok := roomVersions[version]; ok {
		desc = version
	}
	if !desc.Supported {
		err = errors.New("unsupported room version")
	}
	return
}
