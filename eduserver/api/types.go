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

package api

import "github.com/matrix-org/gomatrixserverlib"

const (
	MSigningKeyUpdate = "m.signing_key_update"
)

type TypingEvent struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
	Typing bool   `json:"typing"`
}

type ReceiptEvent struct {
	UserID    string                      `json:"user_id"`
	RoomID    string                      `json:"room_id"`
	EventID   string                      `json:"event_id"`
	Type      string                      `json:"type"`
	Timestamp gomatrixserverlib.Timestamp `json:"timestamp"`
}

type FederationReceiptMRead struct {
	User map[string]FederationReceiptData `json:"m.read"`
}

type FederationReceiptData struct {
	Data     ReceiptTS `json:"data"`
	EventIDs []string  `json:"event_ids"`
}

type ReceiptMRead struct {
	User map[string]ReceiptTS `json:"m.read"`
}

type ReceiptTS struct {
	TS gomatrixserverlib.Timestamp `json:"ts"`
}

type CrossSigningKeyUpdate struct {
	MasterKey      *gomatrixserverlib.CrossSigningKey `json:"master_key,omitempty"`
	SelfSigningKey *gomatrixserverlib.CrossSigningKey `json:"self_signing_key,omitempty"`
	UserID         string                             `json:"user_id"`
}
