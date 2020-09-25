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
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

// DefaultRoomVersion contains the room version that will, by
// default, be used to create new rooms on this server.
func DefaultRoomVersion() gomatrixserverlib.RoomVersion {
	return gomatrixserverlib.RoomVersionV6
}

// RoomVersions returns a map of all known room versions to this
// server.
func RoomVersions() map[gomatrixserverlib.RoomVersion]gomatrixserverlib.RoomVersionDescription {
	return gomatrixserverlib.RoomVersions()
}

// SupportedRoomVersions returns a map of descriptions for room
// versions that are supported by this homeserver.
func SupportedRoomVersions() map[gomatrixserverlib.RoomVersion]gomatrixserverlib.RoomVersionDescription {
	return gomatrixserverlib.SupportedRoomVersions()
}

// RoomVersion returns information about a specific room version.
// An UnknownVersionError is returned if the version is not known
// to the server.
func RoomVersion(version gomatrixserverlib.RoomVersion) (gomatrixserverlib.RoomVersionDescription, error) {
	if version, ok := gomatrixserverlib.RoomVersions()[version]; ok {
		return version, nil
	}
	return gomatrixserverlib.RoomVersionDescription{}, UnknownVersionError{version}
}

// SupportedRoomVersion returns information about a specific room
// version. An UnknownVersionError is returned if the version is not
// known to the server, or an UnsupportedVersionError is returned if
// the version is known but specifically marked as unsupported.
func SupportedRoomVersion(version gomatrixserverlib.RoomVersion) (gomatrixserverlib.RoomVersionDescription, error) {
	result, err := RoomVersion(version)
	if err != nil {
		return gomatrixserverlib.RoomVersionDescription{}, err
	}
	if !result.Supported {
		return gomatrixserverlib.RoomVersionDescription{}, UnsupportedVersionError{version}
	}
	return result, nil
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
