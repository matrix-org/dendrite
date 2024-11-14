// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package authtypes

// Membership represents the relationship between a user and a room they're a
// member of
type Membership struct {
	Localpart string
	RoomID    string
	EventID   string
}
