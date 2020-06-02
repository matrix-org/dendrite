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

	"github.com/matrix-org/dendrite/federationsender/api"
	go_http_js_libp2p "github.com/matrix-org/go-http-js-libp2p"
	"github.com/matrix-org/gomatrixserverlib"
)

type libp2pPublicRoomsProvider struct {
	node      *go_http_js_libp2p.P2pLocalNode
	providers []go_http_js_libp2p.PeerInfo
	fedSender api.FederationSenderInternalAPI
}

func NewLibP2PPublicRoomsProvider(node *go_http_js_libp2p.P2pLocalNode, fedSender api.FederationSenderInternalAPI) *libp2pPublicRoomsProvider {
	p := &libp2pPublicRoomsProvider{
		node:      node,
		fedSender: fedSender,
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

func (p *libp2pPublicRoomsProvider) Homeservers() []string {
	result := make([]string, len(p.providers))
	for i := range p.providers {
		result[i] = p.providers[i].Id
	}
	return result
}
