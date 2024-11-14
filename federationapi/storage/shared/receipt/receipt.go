// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.
// A Receipt contains the NIDs of a call to GetNextTransactionPDUs/EDUs.
// We don't actually export the NIDs but we need the caller to be able
// to pass them back so that we can clean up if the transaction sends
// successfully.

package receipt

import "fmt"

// Receipt is a wrapper type used to represent a nid that corresponds to a unique row entry
// in some database table.
// The internal nid value cannot be modified after a Receipt has been created.
// This guarantees a receipt will always refer to the same table entry that it was created
// to represent.
type Receipt struct {
	nid int64
}

func NewReceipt(nid int64) Receipt {
	return Receipt{nid: nid}
}

func (r *Receipt) GetNID() int64 {
	return r.nid
}

func (r *Receipt) String() string {
	return fmt.Sprintf("%d", r.nid)
}
