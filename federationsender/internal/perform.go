package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/internal/perform"
	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/gomatrixserverlib"
)

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

	// Try to perform a make_join using the information supplied in the
	// request.
	respMakeJoin, err := r.federation.MakeJoin(
		ctx,
		request.ServerName,
		request.RoomID,
		request.UserID,
		supportedVersions,
	)
	if err != nil {
		// TODO: Check if the user was not allowed to join the room.
		return fmt.Errorf("r.federation.MakeJoin: %w", err)
	}

	// Set all the fields to be what they should be, this should be a no-op
	// but it's possible that the remote server returned us something "odd"
	respMakeJoin.JoinEvent.Type = gomatrixserverlib.MRoomMember
	respMakeJoin.JoinEvent.Sender = request.UserID
	respMakeJoin.JoinEvent.StateKey = &request.UserID
	respMakeJoin.JoinEvent.RoomID = request.RoomID
	respMakeJoin.JoinEvent.Redacts = ""
	if request.Content == nil {
		request.Content = map[string]interface{}{}
	}
	request.Content["membership"] = "join"
	if err = respMakeJoin.JoinEvent.SetContent(request.Content); err != nil {
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
		request.ServerName,
		event,
		respMakeJoin.RoomVersion,
	)
	if err != nil {
		return fmt.Errorf("r.federation.SendJoin: %w", err)
	}

	// Check that the send_join response was valid.
	joinCtx := perform.JoinContext(r.federation, r.keyRing)
	if err = joinCtx.CheckSendJoinResponse(
		ctx, event, request.ServerName, respMakeJoin, respSendJoin,
	); err != nil {
		return fmt.Errorf("perform.JoinRequest.CheckSendJoinResponse: %w", err)
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

	// Everything went to plan.
	return nil
}

// PerformLeaveRequest implements api.FederationSenderInternalAPI
func (r *FederationSenderInternalAPI) PerformLeave(
	ctx context.Context,
	request *api.PerformLeaveRequest,
	response *api.PerformLeaveResponse,
) (err error) {
	return nil
}
