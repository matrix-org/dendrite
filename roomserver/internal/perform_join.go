package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/common"
	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// WriteOutputEvents implements OutputRoomEventWriter
func (r *RoomserverInternalAPI) PerformJoin(
	ctx context.Context,
	req *api.PerformJoinRequest,
	res *api.PerformJoinResponse,
) error {
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return fmt.Errorf("supplied user ID %q in incorrect format", req.UserID)
	}
	if domain != r.Cfg.Matrix.ServerName {
		return fmt.Errorf("user ID %q does not belong to this homeserver", req.UserID)
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "!") {
		return r.performJoinRoomByID(ctx, req, res)
	}
	if strings.HasPrefix(req.RoomIDOrAlias, "#") {
		return r.performJoinRoomByAlias(ctx, req, res)
	}
	return fmt.Errorf("unexpected sigil on room %q", req.RoomIDOrAlias)
}

func (r *RoomserverInternalAPI) performJoinRoomByAlias(
	ctx context.Context,
	req *api.PerformJoinRequest,
	res *api.PerformJoinResponse,
) error {
	// Look up if we know this room alias.
	roomID, err := r.DB.GetRoomIDForAlias(ctx, req.RoomIDOrAlias)
	if err != nil {
		return err
	}

	// If we do, then pluck out the room ID and continue the join.
	req.RoomIDOrAlias = roomID
	return r.performJoinRoomByID(ctx, req, res)
}

func (r *RoomserverInternalAPI) performJoinRoomByID(
	ctx context.Context,
	req *api.PerformJoinRequest,
	res *api.PerformJoinResponse,
) error {
	// Prepare the template for the join event.
	userID := req.UserID
	eb := gomatrixserverlib.EventBuilder{
		Type:     gomatrixserverlib.MRoomMember,
		Sender:   userID,
		StateKey: &userID,
		RoomID:   req.RoomIDOrAlias,
		Redacts:  "",
	}
	if err := eb.SetUnsigned(struct{}{}); err != nil {
		return fmt.Errorf("eb.SetUnsigned: %w", err)
	}

	// It is possible for the requestoto include some "content" for the
	// event. We'll always overwrite the "membership" key, but the rest,
	// like "display_name" or "avatar_url", will be kept if supplied.
	if req.Content == nil {
		req.Content = map[string]interface{}{}
	}
	req.Content["membership"] = "join"
	if err := eb.SetContent(req.Content); err != nil {
		return fmt.Errorf("eb.SetContent: %w", err)
	}

	// Try to construct an actual join event from the template.
	// If this succeeds then it is a sign that the room already exists
	// locally on the homeserver.
	// TODO: Check what happens if the room exists on the server
	// but everyone has since left. I suspect it does the wrong thing.
	buildRes := api.QueryLatestEventsAndStateResponse{}
	event, err := common.BuildEvent(
		ctx,        // the request context
		&eb,        // the template join event
		r.Cfg,      // the server configuration
		time.Now(), // the event timestamp to use
		r,          // the roomserver API to use
		&buildRes,  // the query response
	)

	switch err {
	case nil:
		// The room join is local. Send the new join event into the
		// roomserver.
		inputReq := api.InputRoomEventsRequest{
			InputRoomEvents: []api.InputRoomEvent{
				api.InputRoomEvent{
					Kind:         api.KindNew,
					Event:        event.Headered(buildRes.RoomVersion),
					AuthEventIDs: event.AuthEventIDs(),
					SendAsServer: string(r.Cfg.Matrix.ServerName),
				},
			},
		}
		inputRes := api.InputRoomEventsResponse{}
		if err = r.InputRoomEvents(ctx, &inputReq, &inputRes); err != nil {
			return fmt.Errorf("r.InputRoomEvents: %w", err)
		}

	case common.ErrRoomNoExists:
		// The room doesn't exist. First of all check if the room is a local
		// room. If it is then there's nothing more to do - the room just
		// hasn't been created yet.
		if _, domain, derr := gomatrixserverlib.SplitID('!', req.RoomIDOrAlias); derr != nil {
			return fmt.Errorf("room ID %q in incorrect format", req.RoomIDOrAlias)
		} else if domain == r.Cfg.Matrix.ServerName {
			return fmt.Errorf("error trying to join %q room: %w", req.RoomIDOrAlias, derr)
		}

		// Otherwise, if we've reached this point, the room isn't a local room
		// and we should ask the federation sender to try and join for us.
		fedReq := fsAPI.PerformJoinRequest{
			RoomID:     req.RoomIDOrAlias,
			UserID:     req.UserID,
			ServerName: r.Cfg.Matrix.ServerName,
			Content:    req.Content,
		}
		fedRes := fsAPI.PerformJoinResponse{}
		err = r.fsAPI.PerformJoin(ctx, &fedReq, &fedRes)
		if err != nil {
			return fmt.Errorf("error joining federated room %q: %w", req.RoomIDOrAlias, err)
		}

	default:
		return fmt.Errorf("error joining room %q: %w", req.RoomIDOrAlias, err)
	}

	return nil
}
