// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package rooms

import (
	"context"
	"sync"
	"time"

	"github.com/element-hq/dendrite/cmd/dendrite-demo-pinecone/defaults"
	"github.com/element-hq/dendrite/federationapi/api"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"

	pineconeRouter "github.com/matrix-org/pinecone/router"
	pineconeSessions "github.com/matrix-org/pinecone/sessions"
)

type PineconeRoomProvider struct {
	r         *pineconeRouter.Router
	s         *pineconeSessions.Sessions
	fedSender api.FederationInternalAPI
	fedClient fclient.FederationClient
}

func NewPineconeRoomProvider(
	r *pineconeRouter.Router,
	s *pineconeSessions.Sessions,
	fedSender api.FederationInternalAPI,
	fedClient fclient.FederationClient,
) *PineconeRoomProvider {
	p := &PineconeRoomProvider{
		r:         r,
		s:         s,
		fedSender: fedSender,
		fedClient: fedClient,
	}
	return p
}

func (p *PineconeRoomProvider) Rooms() []fclient.PublicRoom {
	list := map[spec.ServerName]struct{}{}
	for k := range defaults.DefaultServerNames {
		list[k] = struct{}{}
	}
	for _, k := range p.r.Peers() {
		list[spec.ServerName(k.PublicKey)] = struct{}{}
	}
	return bulkFetchPublicRoomsFromServers(
		context.Background(), p.fedClient,
		spec.ServerName(p.r.PublicKey().String()), list,
	)
}

// bulkFetchPublicRoomsFromServers fetches public rooms from the list of homeservers.
// Returns a list of public rooms.
func bulkFetchPublicRoomsFromServers(
	ctx context.Context, fedClient fclient.FederationClient,
	origin spec.ServerName,
	homeservers map[spec.ServerName]struct{},
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
	reqctx, reqcancel := context.WithTimeout(ctx, time.Second*5)
	for hs := range homeservers {
		go func(homeserverDomain spec.ServerName) {
			defer wg.Done()
			util.GetLogger(reqctx).WithField("hs", homeserverDomain).Info("Querying HS for public rooms")
			fres, err := fedClient.GetPublicRooms(reqctx, origin, homeserverDomain, int(limit), "", false, "")
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
