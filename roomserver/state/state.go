// Copyright 2017 Vector Creations Ltd
// Copyright 2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package state

import (
	"context"
	"errors"

	"github.com/matrix-org/dendrite/roomserver/state/database"
	v1 "github.com/matrix-org/dendrite/roomserver/state/v1"
	v2 "github.com/matrix-org/dendrite/roomserver/state/v2"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type StateResolutionVersion int

const (
	StateResolutionAlgorithmV1 StateResolutionVersion = iota + 1
	StateResolutionAlgorithmV2
)

func GetStateResolutionAlgorithm(
	version StateResolutionVersion, db database.RoomStateDatabase,
) (StateResolutionImpl, error) {
	switch version {
	case StateResolutionAlgorithmV1:
		return v1.Prepare(db), nil
	case StateResolutionAlgorithmV2:
		return v2.Prepare(db), nil
	default:
		return nil, errors.New("unsupported room version")
	}
}

type StateResolutionImpl interface {
	LoadStateAtSnapshot(ctx context.Context, stateNID types.StateSnapshotNID) ([]types.StateEntry, error)
	LoadStateAtEvent(ctx context.Context, eventID string) ([]types.StateEntry, error)
	LoadCombinedStateAfterEvents(ctx context.Context, prevStates []types.StateAtEvent) ([]types.StateEntry, error)
	DifferenceBetweeenStateSnapshots(ctx context.Context, oldStateNID, newStateNID types.StateSnapshotNID) (removed, added []types.StateEntry, err error)
	LoadStateAtSnapshotForStringTuples(ctx context.Context, stateNID types.StateSnapshotNID, stateKeyTuples []gomatrixserverlib.StateKeyTuple) ([]types.StateEntry, error)
	LoadStateAfterEventsForStringTuples(ctx context.Context, prevStates []types.StateAtEvent, stateKeyTuples []gomatrixserverlib.StateKeyTuple) ([]types.StateEntry, error)
	CalculateAndStoreStateBeforeEvent(ctx context.Context, event gomatrixserverlib.Event, roomNID types.RoomNID) (types.StateSnapshotNID, error)
	CalculateAndStoreStateAfterEvents(ctx context.Context, roomNID types.RoomNID, prevStates []types.StateAtEvent) (types.StateSnapshotNID, error)
}
