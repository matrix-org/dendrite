package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/internal/perform"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/gomatrix"
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

type federatedJoin struct {
	UserID string
	RoomID string
}

// PerformJoin implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformJoin(
	ctx context.Context,
	request *api.PerformJoinRequest,
	response *api.PerformJoinResponse,
) {
	// Check that a join isn't already in progress for this user/room.
	j := federatedJoin{request.UserID, request.RoomID}
	if _, found := r.joins.Load(j); found {
		response.LastError = &gomatrix.HTTPError{
			Code: 429,
			Message: `{
				"errcode": "M_LIMIT_EXCEEDED",
				"error": "There is already a federated join to this room in progress. Please wait for it to finish."
			}`, // TODO: Why do none of our error types play nicely with each other?
		}
		return
	}
	r.joins.Store(j, nil)
	defer r.joins.Delete(j)

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
	var lastErr error
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
			lastErr = err
			continue
		}

		// We're all good.
		return
	}

	// If we reach here then we didn't complete a join for some reason.
	var httpErr gomatrix.HTTPError
	if ok := errors.As(lastErr, &httpErr); ok {
		httpErr.Message = string(httpErr.Contents)
		// Clear the wrapped error, else serialising to JSON (in polylith mode) will fail
		httpErr.WrappedError = nil
		response.LastError = &httpErr
	} else {
		response.LastError = &gomatrix.HTTPError{
			Code:         0,
			WrappedError: nil,
			Message:      "Unknown HTTP error",
		}
		if lastErr != nil {
			response.LastError.Message = lastErr.Error()
		}
	}

	logrus.Errorf(
		"failed to join user %q to room %q through %d server(s): last error %s",
		request.UserID, request.RoomID, len(request.ServerNames), lastErr,
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
	// "If not provided, the room version is assumed to be either "1" or "2"."
	// https://matrix.org/docs/spec/server_server/unstable#get-matrix-federation-v1-make-join-roomid-userid
	if respMakeJoin.RoomVersion == "" {
		respMakeJoin.RoomVersion = setDefaultRoomVersionFromJoinEvent(respMakeJoin.JoinEvent)
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

	// No longer reuse the request context from this point forward.
	// We don't want the client timing out to interrupt the join.
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())

	// Try to perform a send_join using the newly built event.
	respSendJoin, err := r.federation.SendJoin(
		ctx,
		serverName,
		event,
		respMakeJoin.RoomVersion,
	)
	if err != nil {
		r.statistics.ForServer(serverName).Failure()
		cancel()
		return fmt.Errorf("r.federation.SendJoin: %w", err)
	}
	r.statistics.ForServer(serverName).Success()

	// Sanity-check the join response to ensure that it has a create
	// event, that the room version is known, etc.
	if err := sanityCheckSendJoinResponse(respSendJoin); err != nil {
		cancel()
		return fmt.Errorf("sanityCheckSendJoinResponse: %w", err)
	}

	// Process the join response in a goroutine. The idea here is
	// that we'll try and wait for as long as possible for the work
	// to complete, but if the client does give up waiting, we'll
	// still continue to process the join anyway so that we don't
	// waste the effort.
	go func() {
		defer cancel()

		// Check that the send_join response was valid.
		joinCtx := perform.JoinContext(r.federation, r.keyRing)
		respState, err := joinCtx.CheckSendJoinResponse(
			ctx, event, serverName, respMakeJoin, respSendJoin,
		)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"room_id": roomID,
				"user_id": userID,
			}).WithError(err).Error("Failed to process room join response")
			return
		}

		// If we successfully performed a send_join above then the other
		// server now thinks we're a part of the room. Send the newly
		// returned state to the roomserver to update our local view.
		if err = roomserverAPI.SendEventWithState(
			ctx, r.rsAPI,
			roomserverAPI.KindNew,
			respState,
			event.Headered(respMakeJoin.RoomVersion),
			nil,
		); err != nil {
			logrus.WithFields(logrus.Fields{
				"room_id": roomID,
				"user_id": userID,
			}).WithError(err).Error("Failed to send room join response to roomserver")
			return
		}
	}()

	<-ctx.Done()
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

// PerformLeaveRequest implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformInvite(
	ctx context.Context,
	request *api.PerformInviteRequest,
	response *api.PerformInviteResponse,
) (err error) {
	if request.Event.StateKey() == nil {
		return errors.New("invite must be a state event")
	}

	_, destination, err := gomatrixserverlib.SplitID('@', *request.Event.StateKey())
	if err != nil {
		return fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"event_id":     request.Event.EventID(),
		"user_id":      *request.Event.StateKey(),
		"room_id":      request.Event.RoomID(),
		"room_version": request.RoomVersion,
		"destination":  destination,
	}).Info("Sending invite")

	inviteReq, err := gomatrixserverlib.NewInviteV2Request(&request.Event, request.InviteRoomState)
	if err != nil {
		return fmt.Errorf("gomatrixserverlib.NewInviteV2Request: %w", err)
	}

	inviteRes, err := r.federation.SendInviteV2(ctx, destination, inviteReq)
	if err != nil {
		return fmt.Errorf("r.federation.SendInviteV2: %w", err)
	}

	response.Event = inviteRes.Event.Headered(request.RoomVersion)
	return nil
}

// PerformServersAlive implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformServersAlive(
	ctx context.Context,
	request *api.PerformServersAliveRequest,
	response *api.PerformServersAliveResponse,
) (err error) {
	for _, srv := range request.Servers {
		r.queues.RetryServer(srv)
	}

	return nil
}

// PerformServersAlive implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformBroadcastEDU(
	ctx context.Context,
	request *api.PerformBroadcastEDURequest,
	response *api.PerformBroadcastEDUResponse,
) (err error) {
	destinations, err := r.db.GetAllJoinedHosts(ctx)
	if err != nil {
		return fmt.Errorf("r.db.GetAllJoinedHosts: %w", err)
	}
	if len(destinations) == 0 {
		return nil
	}

	logrus.WithContext(ctx).Infof("Sending wake-up EDU to %d destination(s)", len(destinations))

	edu := &gomatrixserverlib.EDU{
		Type:   "org.matrix.dendrite.wakeup",
		Origin: string(r.cfg.Matrix.ServerName),
	}
	if err = r.queues.SendEDU(edu, r.cfg.Matrix.ServerName, destinations); err != nil {
		return fmt.Errorf("r.queues.SendEDU: %w", err)
	}

	wakeReq := &api.PerformServersAliveRequest{
		Servers: destinations,
	}
	wakeRes := &api.PerformServersAliveResponse{}
	if err := r.PerformServersAlive(ctx, wakeReq, wakeRes); err != nil {
		return fmt.Errorf("r.PerformServersAlive: %w", err)
	}

	return nil
}

func sanityCheckSendJoinResponse(respSendJoin gomatrixserverlib.RespSendJoin) error {
	// sanity check we have a create event and it has a known room version
	for _, ev := range respSendJoin.AuthEvents {
		if ev.Type() == gomatrixserverlib.MRoomCreate && ev.StateKeyEquals("") {
			// make sure the room version is known
			content := ev.Content()
			verBody := struct {
				Version string `json:"room_version"`
			}{}
			err := json.Unmarshal(content, &verBody)
			if err != nil {
				return err
			}
			if verBody.Version == "" {
				// https://matrix.org/docs/spec/client_server/r0.6.0#m-room-create
				// The version of the room. Defaults to "1" if the key does not exist.
				verBody.Version = "1"
			}
			knownVersions := gomatrixserverlib.RoomVersions()
			if _, ok := knownVersions[gomatrixserverlib.RoomVersion(verBody.Version)]; !ok {
				return fmt.Errorf("send_join m.room.create event has an unknown room version: %s", verBody.Version)
			}
			return nil
		}
	}
	return fmt.Errorf("send_join response is missing m.room.create event")
}

func setDefaultRoomVersionFromJoinEvent(joinEvent gomatrixserverlib.EventBuilder) gomatrixserverlib.RoomVersion {
	// if auth events are not event references we know it must be v3+
	// we have to do these shenanigans to satisfy sytest, specifically for:
	// "Outbound federation rejects m.room.create events with an unknown room version"
	hasEventRefs := true
	authEvents, ok := joinEvent.AuthEvents.([]interface{})
	if ok {
		if len(authEvents) > 0 {
			_, ok = authEvents[0].(string)
			if ok {
				// event refs are objects, not strings, so we know we must be dealing with a v3+ room.
				hasEventRefs = false
			}
		}
	}

	if hasEventRefs {
		return gomatrixserverlib.RoomVersionV1
	}
	return gomatrixserverlib.RoomVersionV4
}
