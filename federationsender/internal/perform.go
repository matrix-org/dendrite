package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/internal/perform"
	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// PerformLeaveRequest implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformDirectoryLookup(
	ctx context.Context,
	request *api.PerformDirectoryLookupRequest,
	response *api.PerformDirectoryLookupResponse,
) (err error) {
	dir, err := r.federation.LookupRoomAlias(
		ctx,
		request.ServerName,
		request.RoomAlias,
	)
	if err != nil {
		r.statistics.ForServer(request.ServerName).Failure()
		return err
	}
	response.RoomID = dir.RoomID
	response.ServerNames = dir.Servers
	r.statistics.ForServer(request.ServerName).Success()
	return nil
}

// PerformJoinRequest implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) (err error) {
	// Look up the supported room versions.
	var supportedVersions []gomatrixserverlib.RoomVersion
	for version := range version.SupportedRoomVersions() {
		supportedVersions = append(supportedVersions, version)
	}

	// Deduplicate the server names we were provided but keep the ordering
	// as this encodes useful information about which servers are most likely
	// to respond.
	seenSet := make(map[gomatrixserverlib.ServerName]bool)
	var uniqueList []gomatrixserverlib.ServerName
	for _, srv := range request.ServerNames {
		if seenSet[srv] {
			continue
		}
		seenSet[srv] = true
		uniqueList = append(uniqueList, srv)
	}
	request.ServerNames = uniqueList

	// Try each server that we were provided until we land on one that
	// successfully completes the make-join send-join dance.
	for _, serverName := range request.ServerNames {
		if err := r.performJoinUsingServer(
			ctx,
			request.RoomID,
			request.UserID,
			request.Content,
			serverName,
			supportedVersions,
		); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"server_name": serverName,
				"room_id":     request.RoomID,
			}).Warnf("Failed to join room through server")
			continue
		}

		// We're all good.
		return nil
	}

	// If we reach here then we didn't complete a join for some reason.
	return fmt.Errorf(
		"failed to join user %q to room %q through %d server(s)",
		request.UserID, request.RoomID, len(request.ServerNames),
	)
}

func (r *FederationSenderInternalAPI) performJoinUsingServer(
	ctx context.Context,
	roomID, userID string,
	content map[string]interface{},
	serverName gomatrixserverlib.ServerName,
	supportedVersions []gomatrixserverlib.RoomVersion,
) error {
	// Try to perform a make_join using the information supplied in the
	// request.
	respMakeJoin, err := r.federation.MakeJoin(
		ctx,
		serverName,
		roomID,
		userID,
		supportedVersions,
	)
	if err != nil {
		// TODO: Check if the user was not allowed to join the room.
		r.statistics.ForServer(serverName).Failure()
		return fmt.Errorf("r.federation.MakeJoin: %w", err)
	}
	r.statistics.ForServer(serverName).Success()

	// Set all the fields to be what they should be, this should be a no-op
	// but it's possible that the remote server returned us something "odd"
	respMakeJoin.JoinEvent.Type = gomatrixserverlib.MRoomMember
	respMakeJoin.JoinEvent.Sender = userID
	respMakeJoin.JoinEvent.StateKey = &userID
	respMakeJoin.JoinEvent.RoomID = roomID
	respMakeJoin.JoinEvent.Redacts = ""
	if content == nil {
		content = map[string]interface{}{}
	}
	content["membership"] = "join"
	if err = respMakeJoin.JoinEvent.SetContent(content); err != nil {
		return fmt.Errorf("respMakeJoin.JoinEvent.SetContent: %w", err)
	}
	if err = respMakeJoin.JoinEvent.SetUnsigned(struct{}{}); err != nil {
		return fmt.Errorf("respMakeJoin.JoinEvent.SetUnsigned: %w", err)
	}

	// Work out if we support the room version that has been supplied in
	// the make_join response.
	if respMakeJoin.RoomVersion == "" {
		respMakeJoin.RoomVersion = gomatrixserverlib.RoomVersionV1
	}
	if _, err = respMakeJoin.RoomVersion.EventFormat(); err != nil {
		return fmt.Errorf("respMakeJoin.RoomVersion.EventFormat: %w", err)
	}

	// Build the join event.
	event, err := respMakeJoin.JoinEvent.Build(
		time.Now(),
		r.cfg.Matrix.ServerName,
		r.cfg.Matrix.KeyID,
		r.cfg.Matrix.PrivateKey,
		respMakeJoin.RoomVersion,
	)
	if err != nil {
		return fmt.Errorf("respMakeJoin.JoinEvent.Build: %w", err)
	}

	// Try to perform a send_join using the newly built event.
	respSendJoin, err := r.federation.SendJoin(
		ctx,
		serverName,
		event,
		respMakeJoin.RoomVersion,
	)
	if err != nil {
		r.statistics.ForServer(serverName).Failure()
		return fmt.Errorf("r.federation.SendJoin: %w", err)
	}
	r.statistics.ForServer(serverName).Success()

	// Check that the send_join response was valid.
	joinCtx := perform.JoinContext(r.federation, r.keyRing)
	if err = joinCtx.CheckSendJoinResponse(
		ctx, event, serverName, respMakeJoin, respSendJoin,
	); err != nil {
		return fmt.Errorf("joinCtx.CheckSendJoinResponse: %w", err)
	}

	// If we successfully performed a send_join above then the other
	// server now thinks we're a part of the room. Send the newly
	// returned state to the roomserver to update our local view.
	if err = r.producer.SendEventWithState(
		ctx,
		respSendJoin.ToRespState(),
		event.Headered(respMakeJoin.RoomVersion),
	); err != nil {
		return fmt.Errorf("r.producer.SendEventWithState: %w", err)
	}

	return nil
}

// PerformLeaveRequest implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformLeave(
	ctx context.Context,
	request *api.PerformLeaveRequest,
	response *api.PerformLeaveResponse,
) (err error) {
	// Deduplicate the server names we were provided.
	util.SortAndUnique(request.ServerNames)

	// Try each server that we were provided until we land on one that
	// successfully completes the make-leave send-leave dance.
	for _, serverName := range request.ServerNames {
		// Try to perform a make_leave using the information supplied in the
		// request.
		respMakeLeave, err := r.federation.MakeLeave(
			ctx,
			serverName,
			request.RoomID,
			request.UserID,
		)
		if err != nil {
			// TODO: Check if the user was not allowed to leave the room.
			logrus.WithError(err).Warnf("r.federation.MakeLeave failed")
			r.statistics.ForServer(serverName).Failure()
			continue
		}

		// Set all the fields to be what they should be, this should be a no-op
		// but it's possible that the remote server returned us something "odd"
		respMakeLeave.LeaveEvent.Type = gomatrixserverlib.MRoomMember
		respMakeLeave.LeaveEvent.Sender = request.UserID
		respMakeLeave.LeaveEvent.StateKey = &request.UserID
		respMakeLeave.LeaveEvent.RoomID = request.RoomID
		respMakeLeave.LeaveEvent.Redacts = ""
		if respMakeLeave.LeaveEvent.Content == nil {
			content := map[string]interface{}{
				"membership": "leave",
			}
			if err = respMakeLeave.LeaveEvent.SetContent(content); err != nil {
				logrus.WithError(err).Warnf("respMakeLeave.LeaveEvent.SetContent failed")
				continue
			}
		}
		if err = respMakeLeave.LeaveEvent.SetUnsigned(struct{}{}); err != nil {
			logrus.WithError(err).Warnf("respMakeLeave.LeaveEvent.SetUnsigned failed")
			continue
		}

		// Work out if we support the room version that has been supplied in
		// the make_leave response.
		if _, err = respMakeLeave.RoomVersion.EventFormat(); err != nil {
			return gomatrixserverlib.UnsupportedRoomVersionError{}
		}

		// Build the leave event.
		event, err := respMakeLeave.LeaveEvent.Build(
			time.Now(),
			r.cfg.Matrix.ServerName,
			r.cfg.Matrix.KeyID,
			r.cfg.Matrix.PrivateKey,
			respMakeLeave.RoomVersion,
		)
		if err != nil {
			logrus.WithError(err).Warnf("respMakeLeave.LeaveEvent.Build failed")
			continue
		}

		// Try to perform a send_leave using the newly built event.
		err = r.federation.SendLeave(
			ctx,
			serverName,
			event,
		)
		if err != nil {
			logrus.WithError(err).Warnf("r.federation.SendLeave failed")
			r.statistics.ForServer(serverName).Failure()
			continue
		}

		r.statistics.ForServer(serverName).Success()
		return nil
	}

	// If we reach here then we didn't complete a leave for some reason.
	return fmt.Errorf(
		"Failed to leave room %q through %d server(s)",
		request.RoomID, len(request.ServerNames),
	)
}
