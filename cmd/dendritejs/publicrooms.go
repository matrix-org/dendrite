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

// +build wasm

package main

import (
	"context"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/federationsender/api"
	go_http_js_libp2p "github.com/matrix-org/go-http-js-libp2p"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type libp2pPublicRoomsProvider struct {
	node      *go_http_js_libp2p.P2pLocalNode
	providers []go_http_js_libp2p.PeerInfo
	fedSender api.FederationSenderInternalAPI
	fedClient *gomatrixserverlib.FederationClient
}

func NewLibP2PPublicRoomsProvider(
	node *go_http_js_libp2p.P2pLocalNode, fedSender api.FederationSenderInternalAPI, fedClient *gomatrixserverlib.FederationClient,
) *libp2pPublicRoomsProvider {
	p := &libp2pPublicRoomsProvider{
		node:      node,
		fedSender: fedSender,
		fedClient: fedClient,
	}
	node.RegisterFoundProviders(p.foundProviders)
	return p
}

func (p *libp2pPublicRoomsProvider) foundProviders(peerInfos []go_http_js_libp2p.PeerInfo) {
	// work out the diff then poke for new ones
	seen := make(map[string]bool, len(p.providers))
	for _, pr := range p.providers {
		seen[pr.Id] = true
	}
	var newPeers []gomatrixserverlib.ServerName
	for _, pi := range peerInfos {
		if !seen[pi.Id] {
			newPeers = append(newPeers, gomatrixserverlib.ServerName(pi.Id))
		}
	}
	if len(newPeers) > 0 {
		var res api.PerformServersAliveResponse
		// ignore errors, we don't care.
		p.fedSender.PerformServersAlive(context.Background(), &api.PerformServersAliveRequest{
			Servers: newPeers,
		}, &res)
	}

	p.providers = peerInfos
}

func (p *libp2pPublicRoomsProvider) Rooms() []gomatrixserverlib.PublicRoom {
	return bulkFetchPublicRoomsFromServers(context.Background(), p.fedClient, p.homeservers())
}

func (p *libp2pPublicRoomsProvider) homeservers() []string {
	result := make([]string, len(p.providers))
	for i := range p.providers {
		result[i] = p.providers[i].Id
	}
	return result
}

// bulkFetchPublicRoomsFromServers fetches public rooms from the list of homeservers.
// Returns a list of public rooms.
func bulkFetchPublicRoomsFromServers(
	ctx context.Context, fedClient *gomatrixserverlib.FederationClient, homeservers []string,
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
		go func(homeserverDomain string) {
			defer wg.Done()
			util.GetLogger(ctx).WithField("hs", homeserverDomain).Info("Querying HS for public rooms")
			fres, err := fedClient.GetPublicRooms(ctx, gomatrixserverlib.ServerName(homeserverDomain), int(limit), "", false, "")
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
		case <-time.After(15 * time.Second): // we've waited long enough, let's tell the client what we got.
			util.GetLogger(ctx).Info("Waited 15s for federated public rooms, returning early")
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
