// Copyright 2020 New Vector Ltd
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

package perform

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
    "github.com/matrix-org/dendrite/roomserver/internal/helpers"
    "github.com/matrix-org/dendrite/roomserver/internal/query"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
    "github.com/matrix-org/dendrite/roomserver/state"
    "github.com/matrix-org/dendrite/roomserver/storage"
    "github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type HandleRemotePeeker struct {
	DB         storage.Database
	Inputer *input.Inputer
}

// PerformHandleRemotePeek handles peeking into matrix rooms, including over
// federation by talking to the federationsender. called when a remote server
// initiates a /peek over federation.
//
// It should atomically figure out the current state of the room (for the
// response to /peek) while adding the remotepeek to the kafka stream so the
// fed sender can start sending peeked events without a race between the state
// snapshot and the stream of peeked events.
func (r *HandleRemotePeeker) PerformHandleRemotePeek(
    ctx context.Context,
    request *api.PerformHandleRemotePeekRequest,
    response *api.PerformHandleRemotePeekResponse,
) error {
    info, err := r.DB.RoomInfo(ctx, request.RoomID)
    if err != nil {
        return err
    }
    if info == nil || info.IsStub {
        return nil
    }
    response.RoomExists = true
    response.RoomVersion = info.RoomVersion

    var stateEvents []gomatrixserverlib.Event

    // XXX: is this right?
    roomState := state.NewStateResolution(r.DB, *info)

    var currentStateSnapshotNID types.StateSnapshotNID
    _, currentStateSnapshotNID, _, err =
            r.DB.LatestEventIDs(ctx, info.RoomNID)
    if err != nil {
            return err
    }
    var stateEntries []types.StateEntry
    stateEntries, err = roomState.LoadStateAtSnapshot(
            ctx, currentStateSnapshotNID,
    )
    if err != nil {
            return err
    }
    stateEvents, err = helpers.LoadStateEvents(ctx, r.DB, stateEntries)
    if err != nil {
            return err
    }

    // get the auth event IDs for the current state events
    var authEventIDs []string
    for _, se := range stateEvents {
        authEventIDs = append(authEventIDs, se.AuthEventIDs()...)
    }
    authEventIDs = util.UniqueStrings(authEventIDs) // de-dupe

    authEvents, err := query.GetAuthChain(ctx, r.DB.EventsFromIDs, authEventIDs)
    if err != nil {
        return err
    }

    for _, event := range stateEvents {
        response.StateEvents = append(response.StateEvents, event.Headered(info.RoomVersion))
    }

    for _, event := range authEvents {
        response.AuthChainEvents = append(response.AuthChainEvents, event.Headered(info.RoomVersion))
    }

    // FIXME: there's a race here - we really should be atomically telling the
    // federationsender to start sending peek events alongside having captured
    // the current state, but it's unclear if/how we can do that.

	err = r.Inputer.WriteOutputEvents(request.RoomID, []api.OutputEvent{
		{
			Type: api.OutputTypeNewRemotePeek,
			NewRemotePeek: &api.OutputNewRemotePeek{
				RoomID:   request.RoomID,
				PeekID:   request.PeekID,
				ServerName: request.ServerName,
                RenewalInterval: request.RenewalInterval,
			},
		},
	})
    return err
}

