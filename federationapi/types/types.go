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

// QueryRequest is the request for /query
type QueryRequest struct {
	DeviceKeys map[string][]string `json:"device_keys"`
}

// UnsignedDeviceInfo is the struct for UDI
type UnsignedDeviceInfo struct {
	DeviceDisplayName string `json:"device_display_name"`
}

// DeviceKeys has the data of the keys of the device
type DeviceKeys struct {
	UserID     string                       `json:"user_id"`
	DeviceID   string                       `json:"edvice_id"`
	Algorithms []string                     `json:"algorithms"`
	Keys       map[string]string            `json:"keys"`
	Signatures map[string]map[string]string `json:"signatures"`
	Unsigned   UnsignedDeviceInfo           `json:"unsigned"`
}

// QueryResponse is the response for /query
type QueryResponse struct {
	DeviceKeys map[string]DeviceKeys `json:"device_keys"`
}

// KeyHolder structure
type KeyHolder struct {
	UserID,
	DeviceID,
	Signature,
	KeyAlgorithm,
	KeyID,
	Key,
	KeyType string
}
