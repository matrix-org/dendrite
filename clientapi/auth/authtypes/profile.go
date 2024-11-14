// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package authtypes

// Profile represents the profile for a Matrix account.
type Profile struct {
	Localpart   string `json:"local_part"`
	ServerName  string `json:"server_name,omitempty"` // NOTSPEC: only set by Pinecone user provider
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// FullyQualifiedProfile represents the profile for a Matrix account.
type FullyQualifiedProfile struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}
