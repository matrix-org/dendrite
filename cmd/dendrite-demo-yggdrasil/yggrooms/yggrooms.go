// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package yggrooms

import (
	"context"
	"sync"
	"time"

	"github.com/element-hq/dendrite/cmd/dendrite-demo-yggdrasil/yggconn"
	"github.com/element-hq/dendrite/federationapi/api"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type YggdrasilRoomProvider struct {
	node      *yggconn.Node
	fedSender api.FederationInternalAPI
	fedClient fclient.FederationClient
}

func NewYggdrasilRoomProvider(
	node *yggconn.Node, fedSender api.FederationInternalAPI, fedClient fclient.FederationClient,
) *YggdrasilRoomProvider {
	p := &YggdrasilRoomProvider{
		node:      node,
		fedSender: fedSender,
		fedClient: fedClient,
	}
	return p
}

func (p *YggdrasilRoomProvider) Rooms() []fclient.PublicRoom {
	return bulkFetchPublicRoomsFromServers(
		context.Background(), p.fedClient,
		spec.ServerName(p.node.DerivedServerName()),
		p.node.KnownNodes(),
	)
}

// bulkFetchPublicRoomsFromServers fetches public rooms from the list of homeservers.
// Returns a list of public rooms.
func bulkFetchPublicRoomsFromServers(
	ctx context.Context, fedClient fclient.FederationClient,
	origin spec.ServerName,
	homeservers []spec.ServerName,
) (publicRooms []fclient.PublicRoom) {
	limit := 200
	// follow pipeline semantics, see https://blog.golang.org/pipelines for more info.
	// goroutines send rooms to this channel
	roomCh := make(chan fclient.PublicRoom, int(limit))
	// signalling channel to tell goroutines to stop sending rooms and quit
	done := make(chan bool)
	// signalling to say when we can close the room channel
	var wg sync.WaitGroup
	wg.Add(len(homeservers))
	// concurrently query for public rooms
	for _, hs := range homeservers {
		go func(homeserverDomain spec.ServerName) {
			defer wg.Done()
			util.GetLogger(ctx).WithField("hs", homeserverDomain).Info("Querying HS for public rooms")
			fres, err := fedClient.GetPublicRooms(ctx, origin, homeserverDomain, int(limit), "", false, "")
			if err != nil {
				util.GetLogger(ctx).WithError(err).WithField("hs", homeserverDomain).Warn(
					"bulkFetchPublicRoomsFromServers: failed to query hs",
				)
				return
			}
			for _, room := range fres.Chunk {
				// atomically send a room or stop
				select {
				case roomCh <- room:
				case <-done:
					util.GetLogger(ctx).WithError(err).WithField("hs", homeserverDomain).Info("Interrupted whilst sending rooms")
					return
				}
			}
		}(hs)
	}

	// Close the room channel when the goroutines have quit so we don't leak, but don't let it stop the in-flight request.
	// This also allows the request to fail fast if all HSes experience errors as it will cause the room channel to be
	// closed.
	go func() {
		wg.Wait()
		util.GetLogger(ctx).Info("Cleaning up resources")
		close(roomCh)
	}()

	// fan-in results with timeout. We stop when we reach the limit.
FanIn:
	for len(publicRooms) < int(limit) || limit == 0 {
		// add a room or timeout
		select {
		case room, ok := <-roomCh:
			if !ok {
				util.GetLogger(ctx).Info("All homeservers have been queried, returning results.")
				break FanIn
			}
			publicRooms = append(publicRooms, room)
		case <-time.After(5 * time.Second): // we've waited long enough, let's tell the client what we got.
			util.GetLogger(ctx).Info("Waited 5s for federated public rooms, returning early")
			break FanIn
		case <-ctx.Done(): // the client hung up on us, let's stop.
			util.GetLogger(ctx).Info("Client hung up, returning early")
			break FanIn
		}
	}
	// tell goroutines to stop
	close(done)

	return publicRooms
}
