// Copyright 2017 Jan Christian Grünhage
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

package synctypes

import (
	"errors"
)

// Filter is used by clients to specify how the server should filter responses to e.g. sync requests
// Specified by: https://spec.matrix.org/v1.6/client-server-api/#filtering
type Filter struct {
	EventFields []string    `json:"event_fields,omitempty"`
	EventFormat string      `json:"event_format,omitempty"`
	Presence    EventFilter `json:"presence,omitempty"`
	AccountData EventFilter `json:"account_data,omitempty"`
	Room        RoomFilter  `json:"room,omitempty"`
}

// EventFilter is used to define filtering rules for events
type EventFilter struct {
	Limit      int       `json:"limit,omitempty"`
	NotSenders *[]string `json:"not_senders,omitempty"`
	NotTypes   *[]string `json:"not_types,omitempty"`
	Senders    *[]string `json:"senders,omitempty"`
	Types      *[]string `json:"types,omitempty"`
}

// RoomFilter is used to define filtering rules for room-related events
type RoomFilter struct {
	NotRooms     *[]string       `json:"not_rooms,omitempty"`
	Rooms        *[]string       `json:"rooms,omitempty"`
	Ephemeral    RoomEventFilter `json:"ephemeral,omitempty"`
	IncludeLeave bool            `json:"include_leave,omitempty"`
	State        StateFilter     `json:"state,omitempty"`
	Timeline     RoomEventFilter `json:"timeline,omitempty"`
	AccountData  RoomEventFilter `json:"account_data,omitempty"`
}

// StateFilter is used to define filtering rules for state events
type StateFilter struct {
	NotSenders                *[]string `json:"not_senders,omitempty"`
	NotTypes                  *[]string `json:"not_types,omitempty"`
	Senders                   *[]string `json:"senders,omitempty"`
	Types                     *[]string `json:"types,omitempty"`
	LazyLoadMembers           bool      `json:"lazy_load_members,omitempty"`
	IncludeRedundantMembers   bool      `json:"include_redundant_members,omitempty"`
	NotRooms                  *[]string `json:"not_rooms,omitempty"`
	Rooms                     *[]string `json:"rooms,omitempty"`
	Limit                     int       `json:"limit,omitempty"`
	UnreadThreadNotifications bool      `json:"unread_thread_notifications,omitempty"`
	ContainsURL               *bool     `json:"contains_url,omitempty"`
}

// RoomEventFilter is used to define filtering rules for events in rooms
type RoomEventFilter struct {
	Limit                     int       `json:"limit,omitempty"`
	NotSenders                *[]string `json:"not_senders,omitempty"`
	NotTypes                  *[]string `json:"not_types,omitempty"`
	Senders                   *[]string `json:"senders,omitempty"`
	Types                     *[]string `json:"types,omitempty"`
	LazyLoadMembers           bool      `json:"lazy_load_members,omitempty"`
	IncludeRedundantMembers   bool      `json:"include_redundant_members,omitempty"`
	NotRooms                  *[]string `json:"not_rooms,omitempty"`
	Rooms                     *[]string `json:"rooms,omitempty"`
	UnreadThreadNotifications bool      `json:"unread_thread_notifications,omitempty"`
	ContainsURL               *bool     `json:"contains_url,omitempty"`
}

// Validate checks if the filter contains valid property values
func (filter *Filter) Validate() error {
	if filter.EventFormat != "" && filter.EventFormat != "client" && filter.EventFormat != "federation" {
		return errors.New("Bad event_format value. Must be one of [\"client\", \"federation\"]")
	}
	return nil
}

// DefaultFilter returns the default filter used by the Matrix server if no filter is provided in
// the request
func DefaultFilter() Filter {
	return Filter{
		AccountData: DefaultEventFilter(),
		EventFields: nil,
		EventFormat: "client",
		Presence:    DefaultEventFilter(),
		Room: RoomFilter{
			AccountData:  DefaultRoomEventFilter(),
			Ephemeral:    DefaultRoomEventFilter(),
			IncludeLeave: false,
			NotRooms:     nil,
			Rooms:        nil,
			State:        DefaultStateFilter(),
			Timeline:     DefaultRoomEventFilter(),
		},
	}
}

// DefaultEventFilter returns the default event filter used by the Matrix server if no filter is
// provided in the request
func DefaultEventFilter() EventFilter {
	return EventFilter{
		// parity with synapse: https://github.com/matrix-org/synapse/blob/v1.80.0/synapse/api/filtering.py#L336
		Limit:      10,
		NotSenders: nil,
		NotTypes:   nil,
		Senders:    nil,
		Types:      nil,
	}
}

// DefaultStateFilter returns the default state event filter used by the Matrix server if no filter
// is provided in the request
func DefaultStateFilter() StateFilter {
	return StateFilter{
		NotSenders:              nil,
		NotTypes:                nil,
		Senders:                 nil,
		Types:                   nil,
		LazyLoadMembers:         false,
		IncludeRedundantMembers: false,
		NotRooms:                nil,
		Rooms:                   nil,
		ContainsURL:             nil,
	}
}

// DefaultRoomEventFilter returns the default room event filter used by the Matrix server if no
// filter is provided in the request
func DefaultRoomEventFilter() RoomEventFilter {
	return RoomEventFilter{
		// parity with synapse: https://github.com/matrix-org/synapse/blob/v1.80.0/synapse/api/filtering.py#L336
		Limit:       10,
		NotSenders:  nil,
		NotTypes:    nil,
		Senders:     nil,
		Types:       nil,
		NotRooms:    nil,
		Rooms:       nil,
		ContainsURL: nil,
	}
}
