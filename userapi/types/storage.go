// Copyright 2021 The Matrix.org Foundation C.I.C.
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

package types

import (
	"math"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
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
type CrossSigningKeyMap map[fclient.CrossSigningKeyPurpose]gomatrixserverlib.Base64Bytes

// Map of user ID -> key ID -> signature
type CrossSigningSigMap map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes
