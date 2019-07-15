// Copyright 2018 New Vector Ltd
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
	"time"

	"github.com/matrix-org/gomatrixserverlib"
)

// Transaction is the representation of a transaction from the federation API
// See https://matrix.org/docs/spec/server_server/unstable.html for more info.
type Transaction struct {
	// The server_name of the homeserver sending this transaction.
	Origin gomatrixserverlib.ServerName `json:"origin"`
	// POSIX timestamp in milliseconds on originating homeserver when this
	// transaction started.
	OriginServerTS int64 `json:"origin_server_ts"`
	// List of persistent updates to rooms.
	PDUs []gomatrixserverlib.Event `json:"pdus"`
}

// NewTransaction sets the timestamp of a new transaction instance and then
// returns the said instance.
func NewTransaction() Transaction {
	// Retrieve the current timestamp in nanoseconds and make it a milliseconds
	// one.
	ts := time.Now().UnixNano() / int64(time.Millisecond)

	return Transaction{OriginServerTS: ts}
}
