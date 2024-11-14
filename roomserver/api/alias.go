// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package api

import (
	"regexp"
)

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
