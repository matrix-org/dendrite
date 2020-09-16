// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"fmt"
	"strings"

	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
)

type Leaver struct {
	Cfg   *config.RoomServer
	DB    storage.Database
	FSAPI fsAPI.FederationSenderInternalAPI

	Inputer *input.Inputer
}

// WriteOutputEvents implements OutputRoomEventWriter
func (r *Leaver) PerformLeave(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse,
) ([]api.OutputEvent, error) {
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return nil, fmt.Errorf("Supplied user ID %q in incorrect format", req.UserID)
	}
	if domain != r.Cfg.Matrix.ServerName {
		return nil, fmt.Errorf("User %q does not belong to this homeserver", req.UserID)
	}
	if strings.HasPrefix(req.RoomID, "!") {
		return r.performLeaveRoomByID(ctx, req, res)
	}
	return nil, fmt.Errorf("Room ID %q is invalid", req.RoomID)
}

func (r *Leaver) performLeaveRoomByID(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse, // nolint:unparam
) ([]api.OutputEvent, error) {
	// If there's an invite outstanding for the room then respond to
	// that.
	isInvitePending, senderUser, eventID, err := helpers.IsInvitePending(ctx, r.DB, req.RoomID, req.UserID)
	if err == nil && isInvitePending {
		return r.performRejectInvite(ctx, req, res, senderUser, eventID)
	}

	// There's no invite pending, so first of all we want to find out
	// if the room exists and if the user is actually in it.
	latestReq := api.QueryLatestEventsAndStateRequest{
		RoomID: req.RoomID,
		StateToFetch: []gomatrixserverlib.StateKeyTuple{
			{
				EventType: gomatrixserverlib.MRoomMember,
				StateKey:  req.UserID,
			},
		},
	}
	latestRes := api.QueryLatestEventsAndStateResponse{}
	if err = helpers.QueryLatestEventsAndState(ctx, r.DB, &latestReq, &latestRes); err != nil {
		return nil, err
	}
	if !latestRes.RoomExists {
		return nil, fmt.Errorf("Room %q does not exist", req.RoomID)
	}

	// Now let's see if the user is in the room.
	if len(latestRes.StateEvents) == 0 {
		return nil, fmt.Errorf("User %q is not a member of room %q", req.UserID, req.RoomID)
	}
	membership, err := latestRes.StateEvents[0].Membership()
	if err != nil {
		return nil, fmt.Errorf("Error getting membership: %w", err)
	}
	if membership != gomatrixserverlib.Join {
		// TODO: should be able to handle "invite" in this case too, if
		// it's a case of kicking or banning or such
		return nil, fmt.Errorf("User %q is not joined to the room (membership is %q)", req.UserID, membership)
	}

	// Prepare the template for the leave event.
	userID := req.UserID
	eb := gomatrixserverlib.EventBuilder{
		Type:     gomatrixserverlib.MRoomMember,
		Sender:   userID,
		StateKey: &userID,
		RoomID:   req.RoomID,
		Redacts:  "",
	}
	if err = eb.SetContent(map[string]interface{}{"membership": "leave"}); err != nil {
		return nil, fmt.Errorf("eb.SetContent: %w", err)
	}
	if err = eb.SetUnsigned(struct{}{}); err != nil {
		return nil, fmt.Errorf("eb.SetUnsigned: %w", err)
	}

	// We know that the user is in the room at this point so let's build
	// a leave event.
	// TODO: Check what happens if the room exists on the server
	// but everyone has since left. I suspect it does the wrong thing.
	event, buildRes, err := buildEvent(ctx, r.DB, r.Cfg.Matrix, &eb)
	if err != nil {
		return nil, fmt.Errorf("eventutil.BuildEvent: %w", err)
	}

	// Give our leave event to the roomserver input stream. The
	// roomserver will process the membership change and notify
	// downstream automatically.
	inputReq := api.InputRoomEventsRequest{
		InputRoomEvents: []api.InputRoomEvent{
			{
				Kind:         api.KindNew,
				Event:        event.Headered(buildRes.RoomVersion),
				AuthEventIDs: event.AuthEventIDs(),
				SendAsServer: string(r.Cfg.Matrix.ServerName),
			},
		},
	}
	inputRes := api.InputRoomEventsResponse{}
	r.Inputer.InputRoomEvents(ctx, &inputReq, &inputRes)
	if err = inputRes.Err(); err != nil {
		return nil, fmt.Errorf("r.InputRoomEvents: %w", err)
	}

	return nil, nil
}

func (r *Leaver) performRejectInvite(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse, // nolint:unparam
	senderUser, eventID string,
) ([]api.OutputEvent, error) {
	_, domain, err := gomatrixserverlib.SplitID('@', senderUser)
	if err != nil {
		return nil, fmt.Errorf("User ID %q invalid: %w", senderUser, err)
	}

	// Ask the federation sender to perform a federated leave for us.
	leaveReq := fsAPI.PerformLeaveRequest{
		RoomID:      req.RoomID,
		UserID:      req.UserID,
		ServerNames: []gomatrixserverlib.ServerName{domain},
	}
	leaveRes := fsAPI.PerformLeaveResponse{}
	if err := r.FSAPI.PerformLeave(ctx, &leaveReq, &leaveRes); err != nil {
		return nil, err
	}

	// Withdraw the invite, so that the sync API etc are
	// notified that we rejected it.
	return []api.OutputEvent{
		{
			Type: api.OutputTypeRetireInviteEvent,
			RetireInviteEvent: &api.OutputRetireInviteEvent{
				EventID:      eventID,
				Membership:   "leave",
				TargetUserID: req.UserID,
			},
		},
	}, nil
}
