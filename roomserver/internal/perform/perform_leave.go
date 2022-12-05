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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"

	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type Leaver struct {
	Cfg     *config.RoomServer
	DB      storage.Database
	FSAPI   fsAPI.RoomserverFederationAPI
	UserAPI userapi.RoomserverUserAPI
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
		return nil, fmt.Errorf("supplied user ID %q in incorrect format", req.UserID)
	}
	if !r.Cfg.Matrix.IsLocalServerName(domain) {
		return nil, fmt.Errorf("user %q does not belong to this homeserver", req.UserID)
	}
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"room_id": req.RoomID,
		"user_id": req.UserID,
	})
	logger.Info("User requested to leave join")
	if strings.HasPrefix(req.RoomID, "!") {
		output, err := r.performLeaveRoomByID(context.Background(), req, res)
		if err != nil {
			logger.WithError(err).Error("Failed to leave room")
		} else {
			logger.Info("User left room successfully")
		}
		return output, err
	}
	return nil, fmt.Errorf("room ID %q is invalid", req.RoomID)
}

func (r *Leaver) performLeaveRoomByID(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse, // nolint:unparam
) ([]api.OutputEvent, error) {
	// If there's an invite outstanding for the room then respond to
	// that.
	isInvitePending, senderUser, eventID, _, err := helpers.IsInvitePending(ctx, r.DB, req.RoomID, req.UserID)
	if err == nil && isInvitePending {
		_, senderDomain, serr := gomatrixserverlib.SplitID('@', senderUser)
		if serr != nil {
			return nil, fmt.Errorf("sender %q is invalid", senderUser)
		}
		if !r.Cfg.Matrix.IsLocalServerName(senderDomain) {
			return r.performFederatedRejectInvite(ctx, req, res, senderUser, eventID)
		}
		// check that this is not a "server notice room"
		accData := &userapi.QueryAccountDataResponse{}
		if err = r.UserAPI.QueryAccountData(ctx, &userapi.QueryAccountDataRequest{
			UserID:   req.UserID,
			RoomID:   req.RoomID,
			DataType: "m.tag",
		}, accData); err != nil {
			return nil, fmt.Errorf("unable to query account data: %w", err)
		}

		if roomData, ok := accData.RoomAccountData[req.RoomID]; ok {
			tagData, ok := roomData["m.tag"]
			if ok {
				tags := gomatrix.TagContent{}
				if err = json.Unmarshal(tagData, &tags); err != nil {
					return nil, fmt.Errorf("unable to unmarshal tag content")
				}
				if _, ok = tags.Tags["m.server_notice"]; ok {
					// mimic the returned values from Synapse
					res.Message = "You cannot reject this invite"
					res.Code = 403
					return nil, fmt.Errorf("You cannot reject this invite")
				}
			}
		}
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
		return nil, fmt.Errorf("room %q does not exist", req.RoomID)
	}

	// Now let's see if the user is in the room.
	if len(latestRes.StateEvents) == 0 {
		return nil, fmt.Errorf("user %q is not a member of room %q", req.UserID, req.RoomID)
	}
	membership, err := latestRes.StateEvents[0].Membership()
	if err != nil {
		return nil, fmt.Errorf("error getting membership: %w", err)
	}
	if membership != gomatrixserverlib.Join && membership != gomatrixserverlib.Invite {
		return nil, fmt.Errorf("user %q is not joined to the room (membership is %q)", req.UserID, membership)
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

	// Get the sender domain.
	_, senderDomain, serr := r.Cfg.Matrix.SplitLocalID('@', eb.Sender)
	if serr != nil {
		return nil, fmt.Errorf("sender %q is invalid", eb.Sender)
	}

	// We know that the user is in the room at this point so let's build
	// a leave event.
	// TODO: Check what happens if the room exists on the server
	// but everyone has since left. I suspect it does the wrong thing.
	event, buildRes, err := buildEvent(ctx, r.DB, r.Cfg.Matrix, senderDomain, &eb)
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
				Origin:       senderDomain,
				SendAsServer: string(senderDomain),
			},
		},
	}
	inputRes := api.InputRoomEventsResponse{}
	if err = r.Inputer.InputRoomEvents(ctx, &inputReq, &inputRes); err != nil {
		return nil, fmt.Errorf("r.Inputer.InputRoomEvents: %w", err)
	}
	if err = inputRes.Err(); err != nil {
		return nil, fmt.Errorf("r.InputRoomEvents: %w", err)
	}

	return nil, nil
}

func (r *Leaver) performFederatedRejectInvite(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse, // nolint:unparam
	senderUser, eventID string,
) ([]api.OutputEvent, error) {
	_, domain, err := gomatrixserverlib.SplitID('@', senderUser)
	if err != nil {
		return nil, fmt.Errorf("user ID %q invalid: %w", senderUser, err)
	}

	// Ask the federation sender to perform a federated leave for us.
	leaveReq := fsAPI.PerformLeaveRequest{
		RoomID:      req.RoomID,
		UserID:      req.UserID,
		ServerNames: []gomatrixserverlib.ServerName{domain},
	}
	leaveRes := fsAPI.PerformLeaveResponse{}
	if err = r.FSAPI.PerformLeave(ctx, &leaveReq, &leaveRes); err != nil {
		// failures in PerformLeave should NEVER stop us from telling other components like the
		// sync API that the invite was withdrawn. Otherwise we can end up with stuck invites.
		util.GetLogger(ctx).WithError(err).Errorf("failed to PerformLeave, still retiring invite event")
	}

	info, err := r.DB.RoomInfo(ctx, req.RoomID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Errorf("failed to get RoomInfo, still retiring invite event")
	}

	updater, err := r.DB.MembershipUpdater(ctx, req.RoomID, req.UserID, true, info.RoomVersion)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Errorf("failed to get MembershipUpdater, still retiring invite event")
	}
	if updater != nil {
		if err = updater.Delete(); err != nil {
			util.GetLogger(ctx).WithError(err).Errorf("failed to delete membership, still retiring invite event")
			if err = updater.Rollback(); err != nil {
				util.GetLogger(ctx).WithError(err).Errorf("failed to rollback deleting membership, still retiring invite event")
			}
		} else {
			if err = updater.Commit(); err != nil {
				util.GetLogger(ctx).WithError(err).Errorf("failed to commit deleting membership, still retiring invite event")
			}
		}
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
