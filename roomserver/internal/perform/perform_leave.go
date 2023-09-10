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
	"time"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"

	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	rsAPI "github.com/matrix-org/dendrite/roomserver/api"
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
	RSAPI   rsAPI.RoomserverInternalAPI
	UserAPI userapi.RoomserverUserAPI
	Inputer *input.Inputer
}

// WriteOutputEvents implements OutputRoomEventWriter
func (r *Leaver) PerformLeave(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse,
) ([]api.OutputEvent, error) {
	if !r.Cfg.Matrix.IsLocalServerName(req.Leaver.Domain()) {
		return nil, fmt.Errorf("user %q does not belong to this homeserver", req.Leaver.String())
	}
	logger := logrus.WithContext(ctx).WithFields(logrus.Fields{
		"room_id": req.RoomID,
		"user_id": req.Leaver.String(),
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

// nolint:gocyclo
func (r *Leaver) performLeaveRoomByID(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse, // nolint:unparam
) ([]api.OutputEvent, error) {
	roomID, err := spec.NewRoomID(req.RoomID)
	if err != nil {
		return nil, err
	}
	leaver, err := r.RSAPI.QuerySenderIDForUser(ctx, *roomID, req.Leaver)
	if err != nil || leaver == nil {
		return nil, fmt.Errorf("leaver %s has no matching senderID in this room", req.Leaver.String())
	}

	// If there's an invite outstanding for the room then respond to
	// that.
	isInvitePending, senderUser, eventID, _, err := helpers.IsInvitePending(ctx, r.DB, req.RoomID, *leaver)
	if err == nil && isInvitePending {
		sender, serr := r.RSAPI.QueryUserIDForSender(ctx, *roomID, senderUser)
		if serr != nil || sender == nil {
			return nil, fmt.Errorf("sender %q has no matching userID", senderUser)
		}
		if !r.Cfg.Matrix.IsLocalServerName(sender.Domain()) {
			return r.performFederatedRejectInvite(ctx, req, res, *sender, eventID, *leaver)
		}
		// check that this is not a "server notice room"
		accData := &userapi.QueryAccountDataResponse{}
		if err = r.UserAPI.QueryAccountData(ctx, &userapi.QueryAccountDataRequest{
			UserID:   req.Leaver.String(),
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
					return nil, spec.LeaveServerNoticeError()
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
				EventType: spec.MRoomMember,
				StateKey:  string(*leaver),
			},
		},
	}
	latestRes := api.QueryLatestEventsAndStateResponse{}
	if err = helpers.QueryLatestEventsAndState(ctx, r.DB, r.RSAPI, &latestReq, &latestRes); err != nil {
		return nil, err
	}
	if !latestRes.RoomExists {
		return nil, fmt.Errorf("room %q does not exist", req.RoomID)
	}

	// Now let's see if the user is in the room.
	if len(latestRes.StateEvents) == 0 {
		return nil, fmt.Errorf("user %q is not a member of room %q", req.Leaver.String(), req.RoomID)
	}
	membership, err := latestRes.StateEvents[0].Membership()
	if err != nil {
		return nil, fmt.Errorf("error getting membership: %w", err)
	}
	if membership != spec.Join && membership != spec.Invite {
		return nil, fmt.Errorf("user %q is not joined to the room (membership is %q)", req.Leaver.String(), membership)
	}

	// Prepare the template for the leave event.
	senderIDString := string(*leaver)
	proto := gomatrixserverlib.ProtoEvent{
		Type:     spec.MRoomMember,
		SenderID: senderIDString,
		StateKey: &senderIDString,
		RoomID:   req.RoomID,
		Redacts:  "",
	}
	if err = proto.SetContent(map[string]interface{}{"membership": "leave"}); err != nil {
		return nil, fmt.Errorf("eb.SetContent: %w", err)
	}
	if err = proto.SetUnsigned(struct{}{}); err != nil {
		return nil, fmt.Errorf("eb.SetUnsigned: %w", err)
	}

	// We know that the user is in the room at this point so let's build
	// a leave event.
	// TODO: Check what happens if the room exists on the server
	// but everyone has since left. I suspect it does the wrong thing.

	validRoomID, err := spec.NewRoomID(req.RoomID)
	if err != nil {
		return nil, err
	}

	var buildRes rsAPI.QueryLatestEventsAndStateResponse
	identity, err := r.RSAPI.SigningIdentityFor(ctx, *validRoomID, req.Leaver)
	if err != nil {
		return nil, fmt.Errorf("SigningIdentityFor: %w", err)
	}
	event, err := eventutil.QueryAndBuildEvent(ctx, &proto, &identity, time.Now(), r.RSAPI, &buildRes)
	if err != nil {
		return nil, fmt.Errorf("eventutil.QueryAndBuildEvent: %w", err)
	}

	// Give our leave event to the roomserver input stream. The
	// roomserver will process the membership change and notify
	// downstream automatically.
	inputReq := api.InputRoomEventsRequest{
		InputRoomEvents: []api.InputRoomEvent{
			{
				Kind:         api.KindNew,
				Event:        event,
				Origin:       req.Leaver.Domain(),
				SendAsServer: string(req.Leaver.Domain()),
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

func (r *Leaver) performFederatedRejectInvite(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse, // nolint:unparam
	inviteSender spec.UserID, eventID string,
	leaver spec.SenderID,
) ([]api.OutputEvent, error) {
	// Ask the federation sender to perform a federated leave for us.
	leaveReq := fsAPI.PerformLeaveRequest{
		RoomID:      req.RoomID,
		UserID:      req.Leaver.String(),
		ServerNames: []spec.ServerName{inviteSender.Domain()},
	}
	leaveRes := fsAPI.PerformLeaveResponse{}
	if err := r.FSAPI.PerformLeave(ctx, &leaveReq, &leaveRes); err != nil {
		// failures in PerformLeave should NEVER stop us from telling other components like the
		// sync API that the invite was withdrawn. Otherwise we can end up with stuck invites.
		util.GetLogger(ctx).WithError(err).Errorf("failed to PerformLeave, still retiring invite event")
	}

	info, err := r.DB.RoomInfo(ctx, req.RoomID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Errorf("failed to get RoomInfo, still retiring invite event")
	}

	updater, err := r.DB.MembershipUpdater(ctx, req.RoomID, string(leaver), true, info.RoomVersion)
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
				EventID:        eventID,
				RoomID:         req.RoomID,
				Membership:     "leave",
				TargetSenderID: leaver,
			},
		},
	}, nil
}
