package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/federationapi/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// PerformLeaveRequest implements api.FederationInternalAPI
func (r *FederationInternalAPI) PerformDirectoryLookup(
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

// PerformJoin implements api.FederationInternalAPI
func (r *FederationInternalAPI) PerformJoin(
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
		response.JoinedVia = serverName
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

func (r *FederationInternalAPI) performJoinUsingServer(
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

	// Try to perform a send_join using the newly built event.
	respSendJoin, err := r.federation.SendJoin(
		context.Background(),
		serverName,
		event,
		respMakeJoin.RoomVersion,
	)
	if err != nil {
		r.statistics.ForServer(serverName).Failure()
		return fmt.Errorf("r.federation.SendJoin: %w", err)
	}
	r.statistics.ForServer(serverName).Success()

	// Sanity-check the join response to ensure that it has a create
	// event, that the room version is known, etc.
	if err := sanityCheckAuthChain(respSendJoin.AuthEvents); err != nil {
		return fmt.Errorf("sanityCheckAuthChain: %w", err)
	}

	// Process the join response in a goroutine. The idea here is
	// that we'll try and wait for as long as possible for the work
	// to complete, but if the client does give up waiting, we'll
	// still continue to process the join anyway so that we don't
	// waste the effort.
	go func() {
		// TODO: Can we expand Check here to return a list of missing auth
		// events rather than failing one at a time?
		respState, err := respSendJoin.Check(context.Background(), r.keyRing, event, federatedAuthProvider(ctx, r.federation, r.keyRing, serverName))
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
			context.Background(),
			r.rsAPI,
			roomserverAPI.KindNew,
			respState,
			event.Headered(respMakeJoin.RoomVersion),
			serverName,
			nil,
			false,
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

// PerformOutboundPeekRequest implements api.FederationInternalAPI
func (r *FederationInternalAPI) PerformOutboundPeek(
	ctx context.Context,
	request *api.PerformOutboundPeekRequest,
	response *api.PerformOutboundPeekResponse,
) error {
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

	// See if there's an existing outbound peek for this room ID with
	// one of the specified servers.
	if peeks, err := r.db.GetOutboundPeeks(ctx, request.RoomID); err == nil {
		for _, peek := range peeks {
			if _, ok := seenSet[peek.ServerName]; ok {
				return nil
			}
		}
	}

	// Try each server that we were provided until we land on one that
	// successfully completes the peek
	var lastErr error
	for _, serverName := range request.ServerNames {
		if err := r.performOutboundPeekUsingServer(
			ctx,
			request.RoomID,
			serverName,
			supportedVersions,
		); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"server_name": serverName,
				"room_id":     request.RoomID,
			}).Warnf("Failed to peek room through server")
			lastErr = err
			continue
		}

		// We're all good.
		return nil
	}

	// If we reach here then we didn't complete a peek for some reason.
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
			Message:      lastErr.Error(),
		}
	}

	logrus.Errorf(
		"failed to peek room %q through %d server(s): last error %s",
		request.RoomID, len(request.ServerNames), lastErr,
	)

	return lastErr
}

func (r *FederationInternalAPI) performOutboundPeekUsingServer(
	ctx context.Context,
	roomID string,
	serverName gomatrixserverlib.ServerName,
	supportedVersions []gomatrixserverlib.RoomVersion,
) error {
	// create a unique ID for this peek.
	// for now we just use the room ID again. In future, if we ever
	// support concurrent peeks to the same room with different filters
	// then we would need to disambiguate further.
	peekID := roomID

	// check whether we're peeking already to try to avoid needlessly
	// re-peeking on the server. we don't need a transaction for this,
	// given this is a nice-to-have.
	outboundPeek, err := r.db.GetOutboundPeek(ctx, serverName, roomID, peekID)
	if err != nil {
		return err
	}
	renewing := false
	if outboundPeek != nil {
		nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
		if nowMilli > outboundPeek.RenewedTimestamp+outboundPeek.RenewalInterval {
			logrus.Infof("stale outbound peek to %s for %s already exists; renewing", serverName, roomID)
			renewing = true
		} else {
			logrus.Infof("live outbound peek to %s for %s already exists", serverName, roomID)
			return nil
		}
	}

	// Try to perform an outbound /peek using the information supplied in the
	// request.
	respPeek, err := r.federation.Peek(
		ctx,
		serverName,
		roomID,
		peekID,
		supportedVersions,
	)
	if err != nil {
		r.statistics.ForServer(serverName).Failure()
		return fmt.Errorf("r.federation.Peek: %w", err)
	}
	r.statistics.ForServer(serverName).Success()

	// Work out if we support the room version that has been supplied in
	// the peek response.
	if respPeek.RoomVersion == "" {
		respPeek.RoomVersion = gomatrixserverlib.RoomVersionV1
	}
	if _, err = respPeek.RoomVersion.EventFormat(); err != nil {
		return fmt.Errorf("respPeek.RoomVersion.EventFormat: %w", err)
	}

	// we have the peek state now so let's process regardless of whether upstream gives up
	ctx = context.Background()

	respState := respPeek.ToRespState()
	// authenticate the state returned (check its auth events etc)
	// the equivalent of CheckSendJoinResponse()
	if err = sanityCheckAuthChain(respState.AuthEvents); err != nil {
		return fmt.Errorf("sanityCheckAuthChain: %w", err)
	}
	if err = respState.Check(ctx, r.keyRing, federatedAuthProvider(ctx, r.federation, r.keyRing, serverName)); err != nil {
		return fmt.Errorf("error checking state returned from peeking: %w", err)
	}

	// If we've got this far, the remote server is peeking.
	if renewing {
		if err = r.db.RenewOutboundPeek(ctx, serverName, roomID, peekID, respPeek.RenewalInterval); err != nil {
			return err
		}
	} else {
		if err = r.db.AddOutboundPeek(ctx, serverName, roomID, peekID, respPeek.RenewalInterval); err != nil {
			return err
		}
	}

	// logrus.Warnf("got respPeek %#v", respPeek)
	// Send the newly returned state to the roomserver to update our local view.
	if err = roomserverAPI.SendEventWithState(
		ctx, r.rsAPI,
		roomserverAPI.KindNew,
		&respState,
		respPeek.LatestEvent.Headered(respPeek.RoomVersion),
		serverName,
		nil,
		false,
	); err != nil {
		return fmt.Errorf("r.producer.SendEventWithState: %w", err)
	}

	return nil
}

// PerformLeaveRequest implements api.FederationInternalAPI
func (r *FederationInternalAPI) PerformLeave(
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
		"failed to leave room %q through %d server(s)",
		request.RoomID, len(request.ServerNames),
	)
}

// PerformLeaveRequest implements api.FederationInternalAPI
func (r *FederationInternalAPI) PerformInvite(
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

	inviteReq, err := gomatrixserverlib.NewInviteV2Request(request.Event, request.InviteRoomState)
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

// PerformServersAlive implements api.FederationInternalAPI
func (r *FederationInternalAPI) PerformServersAlive(
	ctx context.Context,
	request *api.PerformServersAliveRequest,
	response *api.PerformServersAliveResponse,
) (err error) {
	for _, srv := range request.Servers {
		_ = r.db.RemoveServerFromBlacklist(srv)
		r.queues.RetryServer(srv)
	}

	return nil
}

// PerformServersAlive implements api.FederationInternalAPI
func (r *FederationInternalAPI) PerformBroadcastEDU(
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

func sanityCheckAuthChain(authChain []*gomatrixserverlib.Event) error {
	// sanity check we have a create event and it has a known room version
	for _, ev := range authChain {
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
				return fmt.Errorf("auth chain m.room.create event has an unknown room version: %s", verBody.Version)
			}
			return nil
		}
	}
	return fmt.Errorf("auth chain response is missing m.room.create event")
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

// FederatedAuthProvider is an auth chain provider which fetches events from the server provided
func federatedAuthProvider(
	ctx context.Context, federation *gomatrixserverlib.FederationClient,
	keyRing gomatrixserverlib.JSONVerifier, server gomatrixserverlib.ServerName,
) gomatrixserverlib.AuthChainProvider {
	// A list of events that we have retried, if they were not included in
	// the auth events supplied in the send_join.
	retries := map[string][]*gomatrixserverlib.Event{}

	// Define a function which we can pass to Check to retrieve missing
	// auth events inline. This greatly increases our chances of not having
	// to repeat the entire set of checks just for a missing event or two.
	return func(roomVersion gomatrixserverlib.RoomVersion, eventIDs []string) ([]*gomatrixserverlib.Event, error) {
		returning := []*gomatrixserverlib.Event{}

		// See if we have retry entries for each of the supplied event IDs.
		for _, eventID := range eventIDs {
			// If we've already satisfied a request for this event ID before then
			// just append the results. We won't retry the request.
			if retry, ok := retries[eventID]; ok {
				if retry == nil {
					return nil, fmt.Errorf("missingAuth: not retrying failed event ID %q", eventID)
				}
				returning = append(returning, retry...)
				continue
			}

			// Make a note of the fact that we tried to do something with this
			// event ID, even if we don't succeed.
			retries[eventID] = nil

			// Try to retrieve the event from the server that sent us the send
			// join response.
			tx, txerr := federation.GetEvent(ctx, server, eventID)
			if txerr != nil {
				return nil, fmt.Errorf("missingAuth r.federation.GetEvent: %w", txerr)
			}

			// For each event returned, add it to the set of return events. We
			// also will populate the retries, in case someone asks for this
			// event ID again.
			for _, pdu := range tx.PDUs {
				// Try to parse the event.
				ev, everr := gomatrixserverlib.NewEventFromUntrustedJSON(pdu, roomVersion)
				if everr != nil {
					return nil, fmt.Errorf("missingAuth gomatrixserverlib.NewEventFromUntrustedJSON: %w", everr)
				}

				// Check the signatures of the event.
				if err := ev.VerifyEventSignatures(ctx, keyRing); err != nil {
					return nil, fmt.Errorf("missingAuth VerifyEventSignatures: %w", err)
				}

				// If the event is OK then add it to the results and the retry map.
				returning = append(returning, ev)
				retries[ev.EventID()] = append(retries[ev.EventID()], ev)
			}
		}
		return returning, nil
	}
}
