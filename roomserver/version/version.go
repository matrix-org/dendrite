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

var roomVersions = map[string]RoomVersionDescription{
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

func GetRoomVersions() map[string]RoomVersionDescription {
	return roomVersions
}

func GetSupportedRoomVersions() map[string]RoomVersionDescription {
	versions := make(map[string]RoomVersionDescription)
	for id, version := range GetRoomVersionDescriptions() {
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
