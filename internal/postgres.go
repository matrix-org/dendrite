// Copyright 2020 The Matrix.org Foundation C.I.C.
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

// +build !wasm

package internal

import (
	"errors"

	"github.com/lib/pq"
	"github.com/mattn/go-sqlite3"
)

// IsUniqueConstraintViolationErr returns true if the error is a postgresql unique_violation or sqlite3.ErrConstraint
func IsUniqueConstraintViolationErr(err error) bool {
	pqErr, ok := err.(*pq.Error)
	if ok {
		return pqErr.Code == "23505"
	}
	sqliteErr, ok := err.(*sqlite3.Error)
	if ok {
		return errors.Is(sqliteErr, sqlite3.ErrConstraint)
	}
	return false
}
