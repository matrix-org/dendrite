package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/consumers"
	"github.com/matrix-org/dendrite/federationapi/statistics"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/version"
)

// PerformLeaveRequest implements api.FederationInternalAPI
func (r *FederationInternalAPI) PerformDirectoryLookup(
	ctx context.Context,
	request *api.PerformDirectoryLookupRequest,
	response *api.PerformDirectoryLookupResponse,
) (err error) {
	if !r.shouldAttemptDirectFederation(request.ServerName) {
		return fmt.Errorf("relay servers have no meaningful response for directory lookup.")
	}

	dir, err := r.federation.LookupRoomAlias(
		ctx,
		r.cfg.Matrix.ServerName,
		request.ServerName,
		request.RoomAlias,
	)
	if err != nil {
		r.statistics.ForServer(request.ServerName).Failure()
		return err
	}
	response.RoomID = dir.RoomID
	response.ServerNames = dir.Servers
	r.statistics.ForServer(request.ServerName).Success(statistics.SendDirect)
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

	// Deduplicate the server names we were provided but keep the ordering
	// as this encodes useful information about which servers are most likely
	// to respond.
	seenSet := make(map[spec.ServerName]bool)
	var uniqueList []spec.ServerName
	for _, srv := range request.ServerNames {
		if seenSet[srv] || r.cfg.Matrix.IsLocalServerName(srv) {
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
			request.Unsigned,
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
	serverName spec.ServerName,
	unsigned map[string]interface{},
) error {
	if !r.shouldAttemptDirectFederation(serverName) {
		return fmt.Errorf("relay servers have no meaningful response for join.")
	}

	user, err := spec.NewUserID(userID, true)
	if err != nil {
		return err
	}

	joinInput := gomatrixserverlib.PerformJoinInput{
		UserID:        user,
		RoomID:        roomID,
		ServerName:    serverName,
		Content:       content,
		Unsigned:      unsigned,
		PrivateKey:    r.cfg.Matrix.PrivateKey,
		KeyID:         r.cfg.Matrix.KeyID,
		KeyRing:       r.keyRing,
		EventProvider: federatedEventProvider(ctx, r.federation, r.keyRing, user.Domain(), serverName),
	}
	response, joinErr := gomatrixserverlib.PerformJoin(ctx, r, joinInput)

	if joinErr != nil {
		if !joinErr.Reachable {
			r.statistics.ForServer(joinErr.ServerName).Failure()
		} else {
			r.statistics.ForServer(joinErr.ServerName).Success(statistics.SendDirect)
		}
		return joinErr.Err
	}
	r.statistics.ForServer(serverName).Success(statistics.SendDirect)
	if response == nil {
		return fmt.Errorf("Received nil response from gomatrixserverlib.PerformJoin")
	}

	// We need to immediately update our list of joined hosts for this room now as we are technically
	// joined. We must do this synchronously: we cannot rely on the roomserver output events as they
	// will happen asyncly. If we don't update this table, you can end up with bad failure modes like
	// joining a room, waiting for 200 OK then changing device keys and have those keys not be sent
	// to other servers (this was a cause of a flakey sytest "Local device key changes get to remote servers")
	// The events are trusted now as we performed auth checks above.
	joinedHosts, err := consumers.JoinedHostsFromEvents(response.StateSnapshot.GetStateEvents().TrustedEvents(response.JoinEvent.Version(), false))
	if err != nil {
		return fmt.Errorf("JoinedHostsFromEvents: failed to get joined hosts: %s", err)
	}

	logrus.WithField("room", roomID).Infof("Joined federated room with %d hosts", len(joinedHosts))
	if _, err = r.db.UpdateRoom(context.Background(), roomID, joinedHosts, nil, true); err != nil {
		return fmt.Errorf("UpdatedRoom: failed to update room with joined hosts: %s", err)
	}

	// TODO: Can I change this to not take respState but instead just take an opaque list of events?
	if err = roomserverAPI.SendEventWithState(
		context.Background(),
		r.rsAPI,
		user.Domain(),
		roomserverAPI.KindNew,
		response.StateSnapshot,
		response.JoinEvent.Headered(response.JoinEvent.Version()),
		serverName,
		nil,
		false,
	); err != nil {
		return fmt.Errorf("roomserverAPI.SendEventWithState: %w", err)
	}
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
	seenSet := make(map[spec.ServerName]bool)
	var uniqueList []spec.ServerName
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
	serverName spec.ServerName,
	supportedVersions []gomatrixserverlib.RoomVersion,
) error {
	if !r.shouldAttemptDirectFederation(serverName) {
		return fmt.Errorf("relay servers have no meaningful response for outbound peek.")
	}

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
		r.cfg.Matrix.ServerName,
		serverName,
		roomID,
		peekID,
		supportedVersions,
	)
	if err != nil {
		r.statistics.ForServer(serverName).Failure()
		return fmt.Errorf("r.federation.Peek: %w", err)
	}
	r.statistics.ForServer(serverName).Success(statistics.SendDirect)

	// Work out if we support the room version that has been supplied in
	// the peek response.
	if respPeek.RoomVersion == "" {
		respPeek.RoomVersion = gomatrixserverlib.RoomVersionV1
	}
	if !gomatrixserverlib.KnownRoomVersion(respPeek.RoomVersion) {
		return fmt.Errorf("unknown room version: %s", respPeek.RoomVersion)
	}

	// we have the peek state now so let's process regardless of whether upstream gives up
	ctx = context.Background()

	// authenticate the state returned (check its auth events etc)
	// the equivalent of CheckSendJoinResponse()
	authEvents, stateEvents, err := gomatrixserverlib.CheckStateResponse(
		ctx, &respPeek, respPeek.RoomVersion, r.keyRing, federatedEventProvider(ctx, r.federation, r.keyRing, r.cfg.Matrix.ServerName, serverName),
	)
	if err != nil {
		return fmt.Errorf("error checking state returned from peeking: %w", err)
	}
	if err = checkEventsContainCreateEvent(authEvents); err != nil {
		return fmt.Errorf("sanityCheckAuthChain: %w", err)
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
		ctx, r.rsAPI, r.cfg.Matrix.ServerName,
		roomserverAPI.KindNew,
		// use the authorized state from CheckStateResponse
		&fclient.RespState{
			StateEvents: gomatrixserverlib.NewEventJSONsFromEvents(stateEvents),
			AuthEvents:  gomatrixserverlib.NewEventJSONsFromEvents(authEvents),
		},
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
	_, origin, err := r.cfg.Matrix.SplitLocalID('@', request.UserID)
	if err != nil {
		return err
	}

	// Deduplicate the server names we were provided.
	util.SortAndUnique(request.ServerNames)

	// Try each server that we were provided until we land on one that
	// successfully completes the make-leave send-leave dance.
	for _, serverName := range request.ServerNames {
		if !r.shouldAttemptDirectFederation(serverName) {
			continue
		}

		// Try to perform a make_leave using the information supplied in the
		// request.
		respMakeLeave, err := r.federation.MakeLeave(
			ctx,
			origin,
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

		// Work out if we support the room version that has been supplied in
		// the make_leave response.
		_, err = gomatrixserverlib.GetRoomVersion(respMakeLeave.RoomVersion)
		if err != nil {
			return err
		}

		// Set all the fields to be what they should be, this should be a no-op
		// but it's possible that the remote server returned us something "odd"
		respMakeLeave.LeaveEvent.Type = spec.MRoomMember
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

		// Build the leave event.
		event, err := respMakeLeave.LeaveEvent.Build(
			time.Now(),
			origin,
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
			origin,
			serverName,
			event,
		)
		if err != nil {
			logrus.WithError(err).Warnf("r.federation.SendLeave failed")
			r.statistics.ForServer(serverName).Failure()
			continue
		}

		r.statistics.ForServer(serverName).Success(statistics.SendDirect)
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
	_, origin, err := r.cfg.Matrix.SplitLocalID('@', request.Event.Sender())
	if err != nil {
		return err
	}

	if request.Event.StateKey() == nil {
		return errors.New("invite must be a state event")
	}

	_, destination, err := gomatrixserverlib.SplitID('@', *request.Event.StateKey())
	if err != nil {
		return fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
	}

	// TODO (devon): This should be allowed via a relay. Currently only transactions
	// can be sent to relays. Would need to extend relays to handle invites.
	if !r.shouldAttemptDirectFederation(destination) {
		return fmt.Errorf("relay servers have no meaningful response for invite.")
	}

	logrus.WithFields(logrus.Fields{
		"event_id":     request.Event.EventID(),
		"user_id":      *request.Event.StateKey(),
		"room_id":      request.Event.RoomID(),
		"room_version": request.RoomVersion,
		"destination":  destination,
	}).Info("Sending invite")

	inviteReq, err := fclient.NewInviteV2Request(request.Event, request.InviteRoomState)
	if err != nil {
		return fmt.Errorf("gomatrixserverlib.NewInviteV2Request: %w", err)
	}

	inviteRes, err := r.federation.SendInviteV2(ctx, origin, destination, inviteReq)
	if err != nil {
		return fmt.Errorf("r.federation.SendInviteV2: failed to send invite: %w", err)
	}
	verImpl, err := gomatrixserverlib.GetRoomVersion(request.RoomVersion)
	if err != nil {
		return err
	}

	inviteEvent, err := verImpl.NewEventFromUntrustedJSON(inviteRes.Event)
	if err != nil {
		return fmt.Errorf("r.federation.SendInviteV2 failed to decode event response: %w", err)
	}
	response.Event = inviteEvent.Headered(request.RoomVersion)
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
	r.MarkServersAlive(destinations)

	return nil
}

// PerformWakeupServers implements api.FederationInternalAPI
func (r *FederationInternalAPI) PerformWakeupServers(
	ctx context.Context,
	request *api.PerformWakeupServersRequest,
	response *api.PerformWakeupServersResponse,
) (err error) {
	r.MarkServersAlive(request.ServerNames)
	return nil
}

func (r *FederationInternalAPI) MarkServersAlive(destinations []spec.ServerName) {
	for _, srv := range destinations {
		wasBlacklisted := r.statistics.ForServer(srv).MarkServerAlive()
		r.queues.RetryServer(srv, wasBlacklisted)
	}
}

func checkEventsContainCreateEvent(events []*gomatrixserverlib.Event) error {
	// sanity check we have a create event and it has a known room version
	for _, ev := range events {
		if ev.Type() == spec.MRoomCreate && ev.StateKeyEquals("") {
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
				return fmt.Errorf("m.room.create event has an unknown room version: %s", verBody.Version)
			}
			return nil
		}
	}
	return fmt.Errorf("response is missing m.room.create event")
}

// federatedEventProvider is an event provider which fetches events from the server provided
func federatedEventProvider(
	ctx context.Context, federation fclient.FederationClient,
	keyRing gomatrixserverlib.JSONVerifier, origin, server spec.ServerName,
) gomatrixserverlib.EventProvider {
	// A list of events that we have retried, if they were not included in
	// the auth events supplied in the send_join.
	retries := map[string][]*gomatrixserverlib.Event{}

	// Define a function which we can pass to Check to retrieve missing
	// auth events inline. This greatly increases our chances of not having
	// to repeat the entire set of checks just for a missing event or two.
	return func(roomVersion gomatrixserverlib.RoomVersion, eventIDs []string) ([]*gomatrixserverlib.Event, error) {
		returning := []*gomatrixserverlib.Event{}
		verImpl, err := gomatrixserverlib.GetRoomVersion(roomVersion)
		if err != nil {
			return nil, err
		}

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
			tx, txerr := federation.GetEvent(ctx, origin, server, eventID)
			if txerr != nil {
				return nil, fmt.Errorf("missingAuth r.federation.GetEvent: %w", txerr)
			}

			// For each event returned, add it to the set of return events. We
			// also will populate the retries, in case someone asks for this
			// event ID again.
			for _, pdu := range tx.PDUs {
				// Try to parse the event.
				ev, everr := verImpl.NewEventFromUntrustedJSON(pdu)
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

// P2PQueryRelayServers implements api.FederationInternalAPI
func (r *FederationInternalAPI) P2PQueryRelayServers(
	ctx context.Context,
	request *api.P2PQueryRelayServersRequest,
	response *api.P2PQueryRelayServersResponse,
) error {
	logrus.Infof("Getting relay servers for: %s", request.Server)
	relayServers, err := r.db.P2PGetRelayServersForServer(ctx, request.Server)
	if err != nil {
		return err
	}

	response.RelayServers = relayServers
	return nil
}

// P2PAddRelayServers implements api.FederationInternalAPI
func (r *FederationInternalAPI) P2PAddRelayServers(
	ctx context.Context,
	request *api.P2PAddRelayServersRequest,
	response *api.P2PAddRelayServersResponse,
) error {
	logrus.Infof("Adding relay servers for: %s", request.Server)
	err := r.db.P2PAddRelayServersForServer(ctx, request.Server, request.RelayServers)
	if err != nil {
		return err
	}

	return nil
}

// P2PRemoveRelayServers implements api.FederationInternalAPI
func (r *FederationInternalAPI) P2PRemoveRelayServers(
	ctx context.Context,
	request *api.P2PRemoveRelayServersRequest,
	response *api.P2PRemoveRelayServersResponse,
) error {
	logrus.Infof("Adding relay servers for: %s", request.Server)
	err := r.db.P2PRemoveRelayServersForServer(ctx, request.Server, request.RelayServers)
	if err != nil {
		return err
	}

	return nil
}

func (r *FederationInternalAPI) shouldAttemptDirectFederation(
	destination spec.ServerName,
) bool {
	var shouldRelay bool
	stats := r.statistics.ForServer(destination)
	if stats.AssumedOffline() && len(stats.KnownRelayServers()) > 0 {
		shouldRelay = true
	}

	return !shouldRelay
}
