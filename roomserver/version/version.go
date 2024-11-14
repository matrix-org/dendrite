// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package version

import (
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

// RoomVersions returns a map of all known room versions to this
// server.
func RoomVersions() map[gomatrixserverlib.RoomVersion]gomatrixserverlib.IRoomVersion {
	return gomatrixserverlib.RoomVersions()
}

// SupportedRoomVersions returns a map of descriptions for room
// versions that are supported by this homeserver.
func SupportedRoomVersions() map[gomatrixserverlib.RoomVersion]gomatrixserverlib.IRoomVersion {
	return gomatrixserverlib.RoomVersions()
}

// RoomVersion returns information about a specific room version.
// An UnknownVersionError is returned if the version is not known
// to the server.
func RoomVersion(version gomatrixserverlib.RoomVersion) (gomatrixserverlib.IRoomVersion, error) {
	if version, ok := gomatrixserverlib.RoomVersions()[version]; ok {
		return version, nil
	}
	return nil, UnknownVersionError{version}
}

// SupportedRoomVersion returns information about a specific room
// version. An UnknownVersionError is returned if the version is not
// known to the server, or an UnsupportedVersionError is returned if
// the version is known but specifically marked as unsupported.
func SupportedRoomVersion(version gomatrixserverlib.RoomVersion) (gomatrixserverlib.IRoomVersion, error) {
	return RoomVersion(version)
}

// UnknownVersionError is caused when the room version is not known.
type UnknownVersionError struct {
	Version gomatrixserverlib.RoomVersion
}

func (e UnknownVersionError) Error() string {
	return fmt.Sprintf("room version '%s' is not known", e.Version)
}

// UnsupportedVersionError is caused when the room version is specifically
// marked as unsupported.
type UnsupportedVersionError struct {
	Version gomatrixserverlib.RoomVersion
}

func (e UnsupportedVersionError) Error() string {
	return fmt.Sprintf("room version '%s' is marked as unsupported", e.Version)
}
