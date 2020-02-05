package state

import (
	"context"
	"errors"

	"github.com/matrix-org/dendrite/roomserver/state/database"
	v1 "github.com/matrix-org/dendrite/roomserver/state/v1"

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
	default:
		return nil, errors.New("unsupported room version")
	}
}

type StateResolutionImpl interface {
	LoadStateAtSnapshot(
		ctx context.Context, stateNID types.StateSnapshotNID,
	) ([]types.StateEntry, error)
	LoadStateAtEvent(
		ctx context.Context, eventID string,
	) ([]types.StateEntry, error)
	LoadCombinedStateAfterEvents(
		ctx context.Context, prevStates []types.StateAtEvent,
	) ([]types.StateEntry, error)
	DifferenceBetweeenStateSnapshots(
		ctx context.Context, oldStateNID, newStateNID types.StateSnapshotNID,
	) (removed, added []types.StateEntry, err error)
	LoadStateAtSnapshotForStringTuples(
		ctx context.Context,
		stateNID types.StateSnapshotNID,
		stateKeyTuples []gomatrixserverlib.StateKeyTuple,
	) ([]types.StateEntry, error)
	LoadStateAfterEventsForStringTuples(
		ctx context.Context,
		prevStates []types.StateAtEvent,
		stateKeyTuples []gomatrixserverlib.StateKeyTuple,
	) ([]types.StateEntry, error)
	CalculateAndStoreStateBeforeEvent(
		ctx context.Context,
		event gomatrixserverlib.Event,
		roomNID types.RoomNID,
	) (types.StateSnapshotNID, error)
	CalculateAndStoreStateAfterEvents(
		ctx context.Context,
		roomNID types.RoomNID,
		prevStates []types.StateAtEvent,
	) (types.StateSnapshotNID, error)
}
