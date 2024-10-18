// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package api

import "github.com/matrix-org/gomatrixserverlib/fclient"

// ExtraPublicRoomsProvider provides a way to inject extra published rooms into /publicRooms requests.
type ExtraPublicRoomsProvider interface {
	// Rooms returns the extra rooms. This is called on-demand by clients, so cache appropriately.
	Rooms() []fclient.PublicRoom
}

type RegistrationToken struct {
	Token       *string `json:"token"`
	UsesAllowed *int32  `json:"uses_allowed"`
	Pending     *int32  `json:"pending"`
	Completed   *int32  `json:"completed"`
	ExpiryTime  *int64  `json:"expiry_time"`
}
