// Copyright 2023 The Matrix.org Foundation C.I.C.
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
// A Receipt contains the NIDs of a call to GetNextTransactionPDUs/EDUs.
// We don't actually export the NIDs but we need the caller to be able
// to pass them back so that we can clean up if the transaction sends
// successfully.

package receipt

import "fmt"

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
