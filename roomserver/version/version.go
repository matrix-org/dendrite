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
	EventFormatV1 EventFormatID = iota + 1
	EventFormatV2
	EventFormatV3
)

type RoomVersionDescription struct {
	Supported       bool
	Stable          bool
	StateResolution state.StateResolutionVersion
	EventFormat     EventFormatID
}

func GetRoomVersionDescription(version RoomVersionID) (desc RoomVersionDescription, err error) {
	switch version {
	case RoomVersionV1:
		desc = RoomVersionDescription{
			Supported:       true,
			Stable:          true,
			StateResolution: state.StateResolutionAlgorithmV1,
			EventFormat:     EventFormatV1,
		}
	case RoomVersionV2:
		desc = RoomVersionDescription{
			Supported:       false,
			Stable:          true,
			StateResolution: state.StateResolutionAlgorithmV2,
			EventFormat:     EventFormatV1,
		}
	case RoomVersionV3:
		desc = RoomVersionDescription{
			Supported:       false,
			Stable:          true,
			StateResolution: state.StateResolutionAlgorithmV2,
			EventFormat:     EventFormatV1,
		}
	case RoomVersionV4:
		desc = RoomVersionDescription{
			Supported:       false,
			Stable:          true,
			StateResolution: state.StateResolutionAlgorithmV2,
			EventFormat:     EventFormatV2,
		}
	case RoomVersionV5:
		desc = RoomVersionDescription{
			Supported:       false,
			Stable:          true,
			StateResolution: state.StateResolutionAlgorithmV2,
			EventFormat:     EventFormatV3,
		}
	default:
	}
	if !desc.Supported {
		err = errors.New("unsupported room version")
	}
	return
}
