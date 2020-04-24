package query

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// backfillRequester implements gomatrixserverlib.BackfillRequester
type backfillRequester struct {
	db         storage.Database
	fedClient  *gomatrixserverlib.FederationClient
	thisServer gomatrixserverlib.ServerName

	// per-request state
	servers  []gomatrixserverlib.ServerName
	stateIDs []string
}

func (b *backfillRequester) StateIDsBeforeEvent(ctx context.Context, roomID, atEventID string) ([]string, error) {
	c := gomatrixserverlib.FederatedStateProvider{
		FedClient:      b.fedClient,
		AuthEventsOnly: true,
		Server:         b.servers[0],
	}
	res, err := c.StateIDsAtEvent(ctx, roomID, atEventID)
	b.stateIDs = res
	return res, err
}

func (b *backfillRequester) StateBeforeEvent(ctx context.Context, roomVer gomatrixserverlib.RoomVersion, roomID, atEventID string, eventIDs []string) (map[string]*gomatrixserverlib.Event, error) {
	c := gomatrixserverlib.FederatedStateProvider{
		FedClient:      b.fedClient,
		AuthEventsOnly: true,
		Server:         b.servers[0],
	}
	return c.StateAtEvent(ctx, roomVer, roomID, atEventID, eventIDs)
}

// ServersAtEvent is called when trying to determine which server to request from.
// It returns a list of servers which can be queried for backfill requests. These servers
// will be servers that are in the room already. The entries at the beginning are preferred servers
// and will be tried first. An empty list will fail the request.
func (b *backfillRequester) ServersAtEvent(ctx context.Context, roomID, eventID string) (servers []gomatrixserverlib.ServerName) {
	// getMembershipsBeforeEventNID requires a NID, so retrieving the NID for
	// the event is necessary.
	NIDs, err := b.db.EventNIDs(ctx, []string{eventID})
	if err != nil {
		logrus.WithField("event_id", eventID).WithError(err).Error("ServersAtEvent: failed to get event NID for event")
		return
	}

	// Retrieve all "m.room.member" state events of "join" membership, which
	// contains the list of users in the room before the event, therefore all
	// the servers in it at that moment.
	events, err := getMembershipsBeforeEventNID(ctx, b.db, NIDs[eventID], true)
	if err != nil {
		logrus.WithField("event_id", eventID).WithError(err).Error("ServersAtEvent: failed to get memberships before event")
		return
	}

	// Store the server names in a temporary map to avoid duplicates.
	serverSet := make(map[gomatrixserverlib.ServerName]bool)
	for _, event := range events {
		serverSet[event.Origin()] = true
	}
	for server := range serverSet {
		if server == b.thisServer {
			continue
		}
		servers = append(servers, server)
	}
	b.servers = servers
	return
}

// Backfill performs a backfill request to the given server.
// https://matrix.org/docs/spec/server_server/latest#get-matrix-federation-v1-backfill-roomid
func (b *backfillRequester) Backfill(ctx context.Context, server gomatrixserverlib.ServerName, roomID string, fromEventIDs []string, limit int) (*gomatrixserverlib.Transaction, error) {
	tx, err := b.fedClient.Backfill(ctx, server, roomID, limit, fromEventIDs)
	return &tx, err
}

func (b *backfillRequester) ProvideEvents(roomVer gomatrixserverlib.RoomVersion, eventIDs []string) ([]gomatrixserverlib.Event, error) {
	logrus.Info("backfillRequester.ProvideEvents ", eventIDs)
	ctx := context.Background()
	nidMap, err := b.db.EventNIDs(ctx, eventIDs)
	if err != nil {
		logrus.WithError(err).WithField("event_ids", eventIDs).Error("Failed to find events")
		return nil, err
	}
	eventNIDs := make([]types.EventNID, len(nidMap))
	i := 0
	for _, nid := range nidMap {
		eventNIDs[i] = nid
		i++
	}
	eventsWithNids, err := b.db.Events(ctx, eventNIDs)
	if err != nil {
		logrus.WithError(err).WithField("event_nids", eventNIDs).Error("Failed to load events")
		return nil, err
	}
	events := make([]gomatrixserverlib.Event, len(eventsWithNids))
	for i := range eventsWithNids {
		events[i] = eventsWithNids[i].Event
	}
	logrus.Infof("backfillRequester.ProvideEvents Returning %+v", events)
	return events, nil
}
