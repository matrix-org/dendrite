// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package authtypes

// ThreePID represents a third-party identifier
type ThreePID struct {
	Address     string `json:"address"`
	Medium      string `json:"medium"`
	AddedAt     int64  `json:"added_at"`
	ValidatedAt int64  `json:"validated_at"`
}
