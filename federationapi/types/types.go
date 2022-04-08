// Copyright 2017 Vector Creations Ltd
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
	"github.com/matrix-org/gomatrixserverlib"
)

const MSigningKeyUpdate = "m.signing_key_update" // TODO: move to gomatrixserverlib

// A JoinedHost is a server that is joined to a matrix room.
type JoinedHost struct {
	// The MemberEventID of a m.room.member join event.
	MemberEventID string
	// The domain part of the state key of the m.room.member join event
	ServerName gomatrixserverlib.ServerName
}

type ServerNames []gomatrixserverlib.ServerName

func (s ServerNames) Len() int           { return len(s) }
func (s ServerNames) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s ServerNames) Less(i, j int) bool { return s[i] < s[j] }

// tracks peeks we're performing on another server over federation
type OutboundPeek struct {
	PeekID            string
	RoomID            string
	ServerName        gomatrixserverlib.ServerName
	CreationTimestamp int64
	RenewedTimestamp  int64
	RenewalInterval   int64
}

// tracks peeks other servers are performing on us over federation
type InboundPeek struct {
	PeekID            string
	RoomID            string
	ServerName        gomatrixserverlib.ServerName
	CreationTimestamp int64
	RenewedTimestamp  int64
	RenewalInterval   int64
}

type FederationReceiptMRead struct {
	User map[string]FederationReceiptData `json:"m.read"`
}

type FederationReceiptData struct {
	Data     ReceiptTS `json:"data"`
	EventIDs []string  `json:"event_ids"`
}

type ReceiptTS struct {
	TS gomatrixserverlib.Timestamp `json:"ts"`
}

type Presence struct {
	Push []PresenceContent `json:"push"`
}

type PresenceContent struct {
	CurrentlyActive bool    `json:"currently_active,omitempty"`
	LastActiveAgo   int64   `json:"last_active_ago"`
	Presence        string  `json:"presence"`
	StatusMsg       *string `json:"status_msg,omitempty"`
	UserID          string  `json:"user_id"`
}
