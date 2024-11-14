// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

//go:build wasm
// +build wasm

package sqlutil

import (
	"modernc.org/sqlite"
	lib "modernc.org/sqlite/lib"
)

// IsUniqueConstraintViolationErr returns true if the error is an unique_violation error
func IsUniqueConstraintViolationErr(err error) bool {
	switch e := err.(type) {
	case *sqlite.Error:
		return e.Code() == lib.SQLITE_CONSTRAINT
	}
	return false
}
