// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package types

import (
	"math"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

const (
	// OffsetNewest tells e.g. the database to get the most current data
	OffsetNewest int64 = math.MaxInt64
	// OffsetOldest tells e.g. the database to get the oldest data
	OffsetOldest int64 = 0
)

// KeyTypePurposeToInt maps a purpose to an integer, which is used in the
// database to reduce the amount of space taken up by this column.
var KeyTypePurposeToInt = map[fclient.CrossSigningKeyPurpose]int16{
	fclient.CrossSigningKeyPurposeMaster:      1,
	fclient.CrossSigningKeyPurposeSelfSigning: 2,
	fclient.CrossSigningKeyPurposeUserSigning: 3,
}

// KeyTypeIntToPurpose maps an integer to a purpose, which is used in the
// database to reduce the amount of space taken up by this column.
var KeyTypeIntToPurpose = map[int16]fclient.CrossSigningKeyPurpose{
	1: fclient.CrossSigningKeyPurposeMaster,
	2: fclient.CrossSigningKeyPurposeSelfSigning,
	3: fclient.CrossSigningKeyPurposeUserSigning,
}

// Map of purpose -> public key
type CrossSigningKeyMap map[fclient.CrossSigningKeyPurpose]spec.Base64Bytes

// Map of user ID -> key ID -> signature
type CrossSigningSigMap map[string]map[gomatrixserverlib.KeyID]spec.Base64Bytes
