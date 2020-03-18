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
	"github.com/matrix-org/go-http-js-libp2p/go_http_js_libp2p"
)

type libp2pPublicRoomsProvider struct {
	node      *go_http_js_libp2p.P2pLocalNode
	providers []go_http_js_libp2p.PeerInfo
}

func NewLibP2PPublicRoomsProvider(node *go_http_js_libp2p.P2pLocalNode) *libp2pPublicRoomsProvider {
	p := &libp2pPublicRoomsProvider{
		node: node,
	}
	node.RegisterFoundProviders(p.foundProviders)
	return p
}

func (p *libp2pPublicRoomsProvider) foundProviders(peerInfos []go_http_js_libp2p.PeerInfo) {
	p.providers = peerInfos
}

func (p *libp2pPublicRoomsProvider) Homeservers() []string {
	result := make([]string, len(p.providers))
	for i := range p.providers {
		result[i] = p.providers[i].Id
	}
	return result
}
