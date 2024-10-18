// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package types

import "github.com/matrix-org/gomatrixserverlib/spec"

const MSigningKeyUpdate = "m.signing_key_update" // TODO: move to gomatrixserverlib

// A JoinedHost is a server that is joined to a matrix room.
type JoinedHost struct {
	// The MemberEventID of a m.room.member join event.
	MemberEventID string
	// The domain part of the state key of the m.room.member join event
	ServerName spec.ServerName
}

type ServerNames []spec.ServerName

func (s ServerNames) Len() int           { return len(s) }
func (s ServerNames) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s ServerNames) Less(i, j int) bool { return s[i] < s[j] }

// tracks peeks we're performing on another server over federation
type OutboundPeek struct {
	PeekID            string
	RoomID            string
	ServerName        spec.ServerName
	CreationTimestamp int64
	RenewedTimestamp  int64
	RenewalInterval   int64
}

// tracks peeks other servers are performing on us over federation
type InboundPeek struct {
	PeekID            string
	RoomID            string
	ServerName        spec.ServerName
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
	TS spec.Timestamp `json:"ts"`
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
