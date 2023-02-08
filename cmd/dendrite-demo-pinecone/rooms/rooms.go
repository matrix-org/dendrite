// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package rooms

import (
	"context"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/cmd/dendrite-demo-pinecone/defaults"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"

	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

type PineconeRoomProvider struct {
	r         *pineconeRouter.Router
	s         *pineconeSessions.Sessions
	fedSender api.FederationInternalAPI
	fedClient *gomatrixserverlib.FederationClient
}

func NewPineconeRoomProvider(
	r *pineconeRouter.Router,
	s *pineconeSessions.Sessions,
	fedSender api.FederationInternalAPI,
	fedClient *gomatrixserverlib.FederationClient,
) *PineconeRoomProvider {
	p := &PineconeRoomProvider{
		r:         r,
		s:         s,
		fedSender: fedSender,
		fedClient: fedClient,
	}
	return p
}

func (p *PineconeRoomProvider) Rooms() []gomatrixserverlib.PublicRoom {
	list := map[gomatrixserverlib.ServerName]struct{}{}
	for k := range defaults.DefaultServerNames {
		list[k] = struct{}{}
	}
	for _, k := range p.r.Peers() {
		list[gomatrixserverlib.ServerName(k.PublicKey)] = struct{}{}
	}
	return bulkFetchPublicRoomsFromServers(context.Background(), p.fedClient, list)
}

// bulkFetchPublicRoomsFromServers fetches public rooms from the list of homeservers.
// Returns a list of public rooms.
func bulkFetchPublicRoomsFromServers(
	ctx context.Context, fedClient *gomatrixserverlib.FederationClient,
	homeservers map[gomatrixserverlib.ServerName]struct{},
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
	reqctx, reqcancel := context.WithTimeout(ctx, time.Second*5)
	for hs := range homeservers {
		go func(homeserverDomain gomatrixserverlib.ServerName) {
			defer wg.Done()
			util.GetLogger(reqctx).WithField("hs", homeserverDomain).Info("Querying HS for public rooms")
			fres, err := fedClient.GetPublicRooms(reqctx, homeserverDomain, int(limit), "", false, "")
			if err != nil {
				util.GetLogger(reqctx).WithError(err).WithField("hs", homeserverDomain).Warn(
					"bulkFetchPublicRoomsFromServers: failed to query hs",
				)
				return
			}
			for _, room := range fres.Chunk {
				// atomically send a room or stop
				select {
				case roomCh <- room:
				case <-done:
				case <-reqctx.Done():
					util.GetLogger(reqctx).WithError(err).WithField("hs", homeserverDomain).Info("Interrupted whilst sending rooms")
					return
				}
			}
		}(hs)
	}

	select {
	case <-time.After(5 * time.Second):
	default:
		wg.Wait()
	}
	reqcancel()
	close(done)
	close(roomCh)

	for room := range roomCh {
		publicRooms = append(publicRooms, room)
	}

	return publicRooms
}
