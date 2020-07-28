package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// PerformJoin handles joining matrix rooms, including over federation by talking to the federationsender.
func (r *RoomserverInternalAPI) PerformJoin(
	ctx context.Context,
	req *api.PerformJoinRequest,
	res *api.PerformJoinResponse,
) {
	roomID, err := r.performJoin(ctx, req)
	if err != nil {
		perr, ok := err.(*api.PerformError)
		if ok {
			res.Error = perr
		} else {
			res.Error = &api.PerformError{
				Msg: err.Error(),
			}
		}
	}
	res.RoomID = roomID
}

func (r *RoomserverInternalAPI) performJoin(
	ctx context.Context,
	req *api.PerformJoinRequest,
) (string, error) {
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return "", &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Supplied user ID %q in incorrect format", req.UserID),
		}
	}
	if domain != r.Cfg.Matrix.ServerName {
		return "", &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("User %q does not belong to this homeserver", req.UserID),
		}
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "!") {
		return r.performJoinRoomByID(ctx, req)
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "#") {
		return r.performJoinRoomByAlias(ctx, req)
	}
	return "", &api.PerformError{
		Code: api.PerformErrorBadRequest,
		Msg:  fmt.Sprintf("Room ID or alias %q is invalid", req.RoomIDOrAlias),
	}
}

func (r *RoomserverInternalAPI) performJoinRoomByAlias(
	ctx context.Context,
	req *api.PerformJoinRequest,
) (string, error) {
	// Get the domain part of the room alias.
	_, domain, err := gomatrixserverlib.SplitID('#', req.RoomIDOrAlias)
	if err != nil {
		return "", fmt.Errorf("Alias %q is not in the correct format", req.RoomIDOrAlias)
	}
	req.ServerNames = append(req.ServerNames, domain)

	// Check if this alias matches our own server configuration. If it
	// doesn't then we'll need to try a federated join.
	var roomID string
	if domain != r.Cfg.Matrix.ServerName {
		// The alias isn't owned by us, so we will need to try joining using
		// a remote server.
		dirReq := fsAPI.PerformDirectoryLookupRequest{
			RoomAlias:  req.RoomIDOrAlias, // the room alias to lookup
			ServerName: domain,            // the server to ask
		}
		dirRes := fsAPI.PerformDirectoryLookupResponse{}
		err = r.fsAPI.PerformDirectoryLookup(ctx, &dirReq, &dirRes)
		if err != nil {
			logrus.WithError(err).Errorf("error looking up alias %q", req.RoomIDOrAlias)
			return "", fmt.Errorf("Looking up alias %q over federation failed: %w", req.RoomIDOrAlias, err)
		}
		roomID = dirRes.RoomID
		req.ServerNames = append(req.ServerNames, dirRes.ServerNames...)
	} else {
		// Otherwise, look up if we know this room alias locally.
		roomID, err = r.DB.GetRoomIDForAlias(ctx, req.RoomIDOrAlias)
		if err != nil {
			return "", fmt.Errorf("Lookup room alias %q failed: %w", req.RoomIDOrAlias, err)
		}
	}

	// If the room ID is empty then we failed to look up the alias.
	if roomID == "" {
		return "", fmt.Errorf("Alias %q not found", req.RoomIDOrAlias)
	}

	// If we do, then pluck out the room ID and continue the join.
	req.RoomIDOrAlias = roomID
	return r.performJoinRoomByID(ctx, req)
}

// TODO: Break this function up a bit
// nolint:gocyclo
func (r *RoomserverInternalAPI) performJoinRoomByID(
	ctx context.Context,
	req *api.PerformJoinRequest,
) (string, error) {
	// Get the domain part of the room ID.
	_, domain, err := gomatrixserverlib.SplitID('!', req.RoomIDOrAlias)
	if err != nil {
		return "", &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("Room ID %q is invalid: %s", req.RoomIDOrAlias, err),
		}
	}

	// If the server name in the room ID isn't ours then it's a
	// possible candidate for finding the room via federation. Add
	// it to the list of servers to try.
	if domain != r.Cfg.Matrix.ServerName {
		req.ServerNames = append(req.ServerNames, domain)
	}

	// Prepare the template for the join event.
	userID := req.UserID
	eb := gomatrixserverlib.EventBuilder{
		Type:     gomatrixserverlib.MRoomMember,
		Sender:   userID,
		StateKey: &userID,
		RoomID:   req.RoomIDOrAlias,
		Redacts:  "",
	}
	if err = eb.SetUnsigned(struct{}{}); err != nil {
		return "", fmt.Errorf("eb.SetUnsigned: %w", err)
	}

	// It is possible for the request to include some "content" for the
	// event. We'll always overwrite the "membership" key, but the rest,
	// like "display_name" or "avatar_url", will be kept if supplied.
	if req.Content == nil {
		req.Content = map[string]interface{}{}
	}
	req.Content["membership"] = gomatrixserverlib.Join
	if err = eb.SetContent(req.Content); err != nil {
		return "", fmt.Errorf("eb.SetContent: %w", err)
	}

	// First work out if this is in response to an existing invite
	// from a federated server. If it is then we avoid the situation
	// where we might think we know about a room in the following
	// section but don't know the latest state as all of our users
	// have left.
	isInvitePending, inviteSender, _, err := r.isInvitePending(ctx, req.RoomIDOrAlias, req.UserID)
	if err == nil && isInvitePending {
		// Check if there's an invite pending.
		_, inviterDomain, ierr := gomatrixserverlib.SplitID('@', inviteSender)
		if ierr != nil {
			return "", fmt.Errorf("gomatrixserverlib.SplitID: %w", err)
		}

		// Check that the domain isn't ours. If it's local then we don't
		// need to do anything as our own copy of the room state will be
		// up-to-date.
		if inviterDomain != r.Cfg.Matrix.ServerName {
			// Add the server of the person who invited us to the server list,
			// as they should be a fairly good bet.
			req.ServerNames = append(req.ServerNames, inviterDomain)

			// Perform a federated room join.
			return req.RoomIDOrAlias, r.performFederatedJoinRoomByID(ctx, req)
		}
	}

	// Try to construct an actual join event from the template.
	// If this succeeds then it is a sign that the room already exists
	// locally on the homeserver.
	// TODO: Check what happens if the room exists on the server
	// but everyone has since left. I suspect it does the wrong thing.
	buildRes := api.QueryLatestEventsAndStateResponse{}
	event, err := eventutil.BuildEvent(
		ctx,          // the request context
		&eb,          // the template join event
		r.Cfg.Matrix, // the server configuration
		time.Now(),   // the event timestamp to use
		r,            // the roomserver API to use
		&buildRes,    // the query response
	)

	switch err {
	case nil:
		// The room join is local. Send the new join event into the
		// roomserver. First of all check that the user isn't already
		// a member of the room.
		alreadyJoined := false
		for _, se := range buildRes.StateEvents {
			if membership, merr := se.Membership(); merr == nil {
				if se.StateKey() != nil && *se.StateKey() == *event.StateKey() {
					alreadyJoined = (membership == gomatrixserverlib.Join)
					break
				}
			}
		}

		// If we haven't already joined the room then send an event
		// into the room changing our membership status.
		if !alreadyJoined {
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
			if err = r.InputRoomEvents(ctx, &inputReq, &inputRes); err != nil {
				var notAllowed *gomatrixserverlib.NotAllowed
				if errors.As(err, &notAllowed) {
					return "", &api.PerformError{
						Code: api.PerformErrorNotAllowed,
						Msg:  fmt.Sprintf("InputRoomEvents auth failed: %s", err),
					}
				}
				return "", fmt.Errorf("r.InputRoomEvents: %w", err)
			}
		}

	case eventutil.ErrRoomNoExists:
		// The room doesn't exist locally. If the room ID looks like it should
		// be ours then this probably means that we've nuked our database at
		// some point.
		if domain == r.Cfg.Matrix.ServerName {
			// If there are no more server names to try then give up here.
			// Otherwise we'll try a federated join as normal, since it's quite
			// possible that the room still exists on other servers.
			if len(req.ServerNames) == 0 {
				return "", &api.PerformError{
					Code: api.PerformErrorNoRoom,
					Msg:  fmt.Sprintf("Room ID %q does not exist", req.RoomIDOrAlias),
				}
			}
		}

		// Perform a federated room join.
		return req.RoomIDOrAlias, r.performFederatedJoinRoomByID(ctx, req)

	default:
		// Something else went wrong.
		return "", fmt.Errorf("Error joining local room: %q", err)
	}

	// By this point, if req.RoomIDOrAlias contained an alias, then
	// it will have been overwritten with a room ID by performJoinRoomByAlias.
	// We should now include this in the response so that the CS API can
	// return the right room ID.
	return req.RoomIDOrAlias, nil
}

func (r *RoomserverInternalAPI) performFederatedJoinRoomByID(
	ctx context.Context,
	req *api.PerformJoinRequest,
) error {
	// Try joining by all of the supplied server names.
	fedReq := fsAPI.PerformJoinRequest{
		RoomID:      req.RoomIDOrAlias, // the room ID to try and join
		UserID:      req.UserID,        // the user ID joining the room
		ServerNames: req.ServerNames,   // the server to try joining with
		Content:     req.Content,       // the membership event content
	}
	fedRes := fsAPI.PerformJoinResponse{}
	r.fsAPI.PerformJoin(ctx, &fedReq, &fedRes)
	if fedRes.LastError != nil {
		return &api.PerformError{
			Code:       api.PerformErrRemote,
			Msg:        fedRes.LastError.Message,
			RemoteCode: fedRes.LastError.Code,
		}
	}
	return nil
}
