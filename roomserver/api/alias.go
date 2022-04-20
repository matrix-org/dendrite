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

package api

import "regexp"

// SetRoomAliasRequest is a request to SetRoomAlias
type SetRoomAliasRequest struct {
	// ID of the user setting the alias
	UserID string `json:"user_id"`
	// New alias for the room
	Alias string `json:"alias"`
	// The room ID the alias is referring to
	RoomID string `json:"room_id"`
}

// SetRoomAliasResponse is a response to SetRoomAlias
type SetRoomAliasResponse struct {
	// Does the alias already refer to a room?
	AliasExists bool `json:"alias_exists"`
}

// GetRoomIDForAliasRequest is a request to GetRoomIDForAlias
type GetRoomIDForAliasRequest struct {
	// Alias we want to lookup
	Alias string `json:"alias"`
	// Should we ask appservices for their aliases as a part of
	// the request?
	IncludeAppservices bool `json:"include_appservices"`
}

// GetRoomIDForAliasResponse is a response to GetRoomIDForAlias
type GetRoomIDForAliasResponse struct {
	// The room ID the alias refers to
	RoomID string `json:"room_id"`
}

// GetAliasesForRoomIDRequest is a request to GetAliasesForRoomID
type GetAliasesForRoomIDRequest struct {
	// The room ID we want to find aliases for
	RoomID string `json:"room_id"`
}

// GetAliasesForRoomIDResponse is a response to GetAliasesForRoomID
type GetAliasesForRoomIDResponse struct {
	// The aliases the alias refers to
	Aliases []string `json:"aliases"`
}

// GetCreatorIDForAliasRequest is a request to GetCreatorIDForAlias
type GetCreatorIDForAliasRequest struct {
	// The alias we want to find the creator of
	Alias string `json:"alias"`
}

// GetCreatorIDForAliasResponse is a response to GetCreatorIDForAlias
type GetCreatorIDForAliasResponse struct {
	// The user ID of the alias creator
	UserID string `json:"user_id"`
}

// RemoveRoomAliasRequest is a request to RemoveRoomAlias
type RemoveRoomAliasRequest struct {
	// ID of the user removing the alias
	UserID string `json:"user_id"`
	// The room alias to remove
	Alias string `json:"alias"`
}

// RemoveRoomAliasResponse is a response to RemoveRoomAlias
type RemoveRoomAliasResponse struct {
	// Did the alias exist before?
	Found bool `json:"found"`
	// Did we remove it?
	Removed bool `json:"removed"`
}

type AliasEvent struct {
	Alias      string   `json:"alias"`
	AltAliases []string `json:"alt_aliases"`
}

var validateAliasRegex = regexp.MustCompile("^#.*:.+$")

func (a AliasEvent) Valid() bool {
	for _, alias := range a.AltAliases {
		if !validateAliasRegex.MatchString(alias) {
			return false
		}
	}
	return a.Alias == "" || validateAliasRegex.MatchString(a.Alias)
}
