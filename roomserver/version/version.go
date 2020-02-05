package version

import (
	"errors"

	"github.com/matrix-org/dendrite/roomserver/state"
)

type RoomVersionID int

const (
	RoomVersionV1 RoomVersionID = iota + 1
)

type RoomVersionDescription struct {
	Stable          bool
	StateResolution state.StateResolutionVersion
}

func GetRoomVersionDescription(version RoomVersionID) (RoomVersionDescription, error) {
	switch version {
	case RoomVersionV1:
		return RoomVersionDescription{
			Stable:          true,
			StateResolution: state.StateResolutionV1,
		}, nil
	default:
		return nil, errors.New("unsupported room version")
	}
}
