package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// WriteOutputEvents implements OutputRoomEventWriter
func (r *RoomserverInternalAPI) PerformLeave(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse,
) error {
	_, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return fmt.Errorf("Supplied user ID %q in incorrect format", req.UserID)
	}
	if domain != r.Cfg.Matrix.ServerName {
		return fmt.Errorf("User %q does not belong to this homeserver", req.UserID)
	}
	if strings.HasPrefix(req.RoomID, "!") {
		return r.performLeaveRoomByID(ctx, req, res)
	}
	return fmt.Errorf("Room ID %q is invalid", req.RoomID)
}

func (r *RoomserverInternalAPI) performLeaveRoomByID(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse, // nolint:unparam
) error {
	// If there's an invite outstanding for the room then respond to
	// that.
	isInvitePending, senderUser, eventID, err := r.isInvitePending(ctx, req.RoomID, req.UserID)
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
	if err = r.QueryLatestEventsAndState(ctx, &latestReq, &latestRes); err != nil {
		return err
	}
	if !latestRes.RoomExists {
		return fmt.Errorf("Room %q does not exist", req.RoomID)
	}

	// Now let's see if the user is in the room.
	if len(latestRes.StateEvents) == 0 {
		return fmt.Errorf("User %q is not a member of room %q", req.UserID, req.RoomID)
	}
	membership, err := latestRes.StateEvents[0].Membership()
	if err != nil {
		return fmt.Errorf("Error getting membership: %w", err)
	}
	if membership != gomatrixserverlib.Join {
		// TODO: should be able to handle "invite" in this case too, if
		// it's a case of kicking or banning or such
		return fmt.Errorf("User %q is not joined to the room (membership is %q)", req.UserID, membership)
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
		return fmt.Errorf("eb.SetContent: %w", err)
	}
	if err = eb.SetUnsigned(struct{}{}); err != nil {
		return fmt.Errorf("eb.SetUnsigned: %w", err)
	}

	// We know that the user is in the room at this point so let's build
	// a leave event.
	// TODO: Check what happens if the room exists on the server
	// but everyone has since left. I suspect it does the wrong thing.
	buildRes := api.QueryLatestEventsAndStateResponse{}
	event, err := eventutil.BuildEvent(
		ctx,          // the request context
		&eb,          // the template leave event
		r.Cfg.Matrix, // the server configuration
		time.Now(),   // the event timestamp to use
		r,            // the roomserver API to use
		&buildRes,    // the query response
	)
	if err != nil {
		return fmt.Errorf("eventutil.BuildEvent: %w", err)
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
	if err = r.InputRoomEvents(ctx, &inputReq, &inputRes); err != nil {
		return fmt.Errorf("r.InputRoomEvents: %w", err)
	}

	return nil
}

func (r *RoomserverInternalAPI) performRejectInvite(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse, // nolint:unparam
	senderUser, eventID string,
) error {
	_, domain, err := gomatrixserverlib.SplitID('@', senderUser)
	if err != nil {
		return fmt.Errorf("User ID %q invalid: %w", senderUser, err)
	}

	// Ask the federation sender to perform a federated leave for us.
	leaveReq := fsAPI.PerformLeaveRequest{
		RoomID:      req.RoomID,
		UserID:      req.UserID,
		ServerNames: []gomatrixserverlib.ServerName{domain},
	}
	leaveRes := fsAPI.PerformLeaveResponse{}
	if err := r.fsAPI.PerformLeave(ctx, &leaveReq, &leaveRes); err != nil {
		return err
	}

	// Withdraw the invite, so that the sync API etc are
	// notified that we rejected it.
	return r.WriteOutputEvents(req.RoomID, []api.OutputEvent{
		{
			Type: api.OutputTypeRetireInviteEvent,
			RetireInviteEvent: &api.OutputRetireInviteEvent{
				EventID:      eventID,
				Membership:   "leave",
				TargetUserID: req.UserID,
			},
		},
	})
}

func (r *RoomserverInternalAPI) isInvitePending(
	ctx context.Context,
	roomID, userID string,
) (bool, string, string, error) {
	// Look up the room NID for the supplied room ID.
	roomNID, err := r.DB.RoomNID(ctx, roomID)
	if err != nil {
		return false, "", "", fmt.Errorf("r.DB.RoomNID: %w", err)
	}

	// Look up the state key NID for the supplied user ID.
	targetUserNIDs, err := r.DB.EventStateKeyNIDs(ctx, []string{userID})
	if err != nil {
		return false, "", "", fmt.Errorf("r.DB.EventStateKeyNIDs: %w", err)
	}
	targetUserNID, targetUserFound := targetUserNIDs[userID]
	if !targetUserFound {
		return false, "", "", fmt.Errorf("missing NID for user %q (%+v)", userID, targetUserNIDs)
	}

	// Let's see if we have an event active for the user in the room. If
	// we do then it will contain a server name that we can direct the
	// send_leave to.
	senderUserNIDs, eventIDs, err := r.DB.GetInvitesForUser(ctx, roomNID, targetUserNID)
	if err != nil {
		return false, "", "", fmt.Errorf("r.DB.GetInvitesForUser: %w", err)
	}
	if len(senderUserNIDs) == 0 {
		return false, "", "", nil
	}
	userNIDToEventID := make(map[types.EventStateKeyNID]string)
	for i, nid := range senderUserNIDs {
		userNIDToEventID[nid] = eventIDs[i]
	}

	// Look up the user ID from the NID.
	senderUsers, err := r.DB.EventStateKeys(ctx, senderUserNIDs)
	if err != nil {
		return false, "", "", fmt.Errorf("r.DB.EventStateKeys: %w", err)
	}
	if len(senderUsers) == 0 {
		return false, "", "", fmt.Errorf("no senderUsers")
	}

	senderUser, senderUserFound := senderUsers[senderUserNIDs[0]]
	if !senderUserFound {
		return false, "", "", fmt.Errorf("missing user for NID %d (%+v)", senderUserNIDs[0], senderUsers)
	}

	return true, senderUser, userNIDToEventID[senderUserNIDs[0]], nil
}
