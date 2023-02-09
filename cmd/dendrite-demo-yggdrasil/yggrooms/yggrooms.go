// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package yggrooms

import (
	"context"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/cmd/dendrite-demo-yggdrasil/yggconn"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type YggdrasilRoomProvider struct {
	node      *yggconn.Node
	fedSender api.FederationInternalAPI
	fedClient *gomatrixserverlib.FederationClient
}

func NewYggdrasilRoomProvider(
	node *yggconn.Node, fedSender api.FederationInternalAPI, fedClient *gomatrixserverlib.FederationClient,
) *YggdrasilRoomProvider {
	p := &YggdrasilRoomProvider{
		node:      node,
		fedSender: fedSender,
		fedClient: fedClient,
	}
	return p
}

func (p *YggdrasilRoomProvider) Rooms() []gomatrixserverlib.PublicRoom {
	return bulkFetchPublicRoomsFromServers(
		context.Background(), p.fedClient,
		gomatrixserverlib.ServerName(p.node.DerivedServerName()),
		p.node.KnownNodes(),
	)
}

// bulkFetchPublicRoomsFromServers fetches public rooms from the list of homeservers.
// Returns a list of public rooms.
func bulkFetchPublicRoomsFromServers(
	ctx context.Context, fedClient *gomatrixserverlib.FederationClient,
	origin gomatrixserverlib.ServerName,
	homeservers []gomatrixserverlib.ServerName,
) (publicRooms []gomatrixserverlib.PublicRoom) {
	limit := 200
	// follow pipeline semantics, see https://blog.golang.org/pipelines for more info.
	// goroutines send rooms to this channel
	roomCh := make(chan gomatrixserverlib.PublicRoom, int(limit))
	// signalling channel to tell goroutines to stop sending rooms and quit
	done := make(chan bool)
	// signalling to say when we can close the room channel
	var wg sync.WaitGroup
	wg.Add(len(homeservers))
	// concurrently query for public rooms
	for _, hs := range homeservers {
		go func(homeserverDomain gomatrixserverlib.ServerName) {
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
