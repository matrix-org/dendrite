// Copyright 2024 New Vector Ltd.
// Copyright 2017 Andrew Morgan <andrew@amorgan.xyz>
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package authtypes

// Flow represents one possible way that the client can authenticate a request.
// https://matrix.org/docs/spec/client_server/r0.3.0.html#user-interactive-authentication-api
type Flow struct {
	Stages []LoginType `json:"stages"`
}
