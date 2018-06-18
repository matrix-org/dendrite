/* Copyright 2018 New Vector Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gomatrixserverlib

// ApplicationServiceUnsigned is the contents of the unsigned field of an
// ApplicationServiceEvent.
type ApplicationServiceUnsigned struct {
	Age int64 `json:"age,omitempty"`
}

// ApplicationServiceEvent is an event format that is sent off to an
// application service as part of a transaction.
type ApplicationServiceEvent struct {
	Unsigned              ApplicationServiceUnsigned `json:"unsigned,omitempty"`
	Content               RawJSON                    `json:"content,omitempty"`
	EventID               string                     `json:"event_id,omitempty"`
	OriginServerTimestamp int64                      `json:"origin_server_ts,omitempty"`
	RoomID                string                     `json:"room_id,omitempty"`
	Sender                string                     `json:"sender,omitempty"`
	Type                  string                     `json:"type,omitempty"`
	UserID                string                     `json:"user_id,omitempty"`
}

// ApplicationServiceTransaction is the transaction that is sent off to an
// application service.
type ApplicationServiceTransaction struct {
	Events []ApplicationServiceEvent `json:"events"`
}
