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

package events

import (
	"errors"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// ErrRoomNoExists is returned when trying to lookup the state of a room that
// doesn't exist
var ErrRoomNoExists = errors.New("Room does not exist")

// BuildEvent builds a Matrix event using the event builder and roomserver query
// API client provided. If also fills roomserver query API response (if provided)
// in case the function calling FillBuilder needs to use it.
// Returns ErrRoomNoExists if the state of the room could not be retrieved because
// the room doesn't exist
// Returns an error if something else went wrong
func BuildEvent(
	builder *gomatrixserverlib.EventBuilder, cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI, queryRes *api.QueryLatestEventsAndStateResponse,
) (*gomatrixserverlib.Event, error) {
	eventsNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(builder)
	if err != nil {
		return nil, err
	}

	// Ask the roomserver for information about this room
	queryReq := api.QueryLatestEventsAndStateRequest{
		RoomID:       builder.RoomID,
		StateToFetch: eventsNeeded.Tuples(),
	}
	if queryRes == nil {
		queryRes = &api.QueryLatestEventsAndStateResponse{}
	}
	if queryErr := queryAPI.QueryLatestEventsAndState(&queryReq, queryRes); queryErr != nil {
		return nil, err
	}

	if !queryRes.RoomExists {
		return nil, ErrRoomNoExists
	}

	builder.Depth = queryRes.Depth
	builder.PrevEvents = queryRes.LatestEvents

	authEvents := gomatrixserverlib.NewAuthEvents(nil)

	for i := range queryRes.StateEvents {
		authEvents.AddEvent(&queryRes.StateEvents[i])
	}

	refs, err := eventsNeeded.AuthEventReferences(&authEvents)
	if err != nil {
		return nil, err
	}
	builder.AuthEvents = refs

	eventID := fmt.Sprintf("$%s:%s", util.RandomString(16), cfg.Matrix.ServerName)
	now := time.Now()
	event, err := builder.Build(eventID, now, cfg.Matrix.ServerName, cfg.Matrix.KeyID, cfg.Matrix.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &event, nil
}
