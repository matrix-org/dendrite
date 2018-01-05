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

package storage

import (
	"github.com/matrix-org/gomatrix"
)

func isRoomFiltered(roomID string, filter *gomatrix.Filter, filterPart *gomatrix.FilterPart) bool {
	if filter != nil {
		if filter.Room.Rooms != nil && !hasValue(roomID, filter.Room.Rooms) {
			return true
		}
		if filter.Room.NotRooms != nil && hasValue(roomID, filter.Room.NotRooms) {
			return true
		}
	}
	if filterPart != nil {
		if filterPart.Rooms != nil && !hasValue(roomID, filterPart.Rooms) {
			return true
		}
		if filterPart.NotRooms != nil && hasValue(roomID, filterPart.NotRooms) {
			return true
		}
	}
	return false
}

func hasValue(value string, list []string) bool {
	for i := range list {
		if list[i] == value {
			return true
		}
	}
	return false
}
