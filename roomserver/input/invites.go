package input

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

func buildInviteStrippedState(
	ctx context.Context,
	db RoomEventDatabase,
	input api.InputRoomEvent,
) (json.RawMessage, error) {
	roomNID, err := db.RoomNID(ctx, input.Event.RoomID())
	if err != nil || roomNID == 0 {
		return nil, nil
	}
	stateWanted := []gomatrixserverlib.StateKeyTuple{}
	for _, t := range []string{
		gomatrixserverlib.MRoomName, gomatrixserverlib.MRoomCanonicalAlias,
		gomatrixserverlib.MRoomAliases, gomatrixserverlib.MRoomJoinRules,
	} {
		stateWanted = append(stateWanted, gomatrixserverlib.StateKeyTuple{
			EventType: t,
			StateKey:  "",
		})
	}
	_, currentStateSnapshotNID, _, err := db.LatestEventIDs(ctx, roomNID)
	if err != nil {
		return nil, err
	}
	roomState := state.NewStateResolution(db)
	stateEntries, err := roomState.LoadStateAtSnapshotForStringTuples(
		ctx, currentStateSnapshotNID, stateWanted,
	)
	if err != nil {
		return nil, err
	}
	stateNIDs := []types.EventNID{}
	for _, stateNID := range stateEntries {
		stateNIDs = append(stateNIDs, stateNID.EventNID)
	}
	stateEvents, err := db.Events(ctx, stateNIDs)
	if err != nil {
		return nil, err
	}
	inviteState := []gomatrixserverlib.InviteV2StrippedState{}
	for _, event := range stateEvents {
		inviteState = append(inviteState, gomatrixserverlib.NewInviteV2StrippedState(&event.Event))
	}
	inviteStrippedState, err := json.Marshal(inviteState)
	if err != nil {
		return nil, err
	}
	return inviteStrippedState, nil
}
