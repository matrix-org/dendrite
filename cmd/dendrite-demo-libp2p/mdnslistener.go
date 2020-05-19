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

package main

import (
	"context"
	"fmt"
	"math"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/matrix-org/gomatrixserverlib"
)

type mDNSListener struct {
	keydb gomatrixserverlib.KeyDatabase
	host  host.Host
}

func (n *mDNSListener) HandlePeerFound(p peer.AddrInfo) {
	if err := n.host.Connect(context.Background(), p); err != nil {
		fmt.Println("Error adding peer", p.ID.String(), "via mDNS:", err)
	}
	if pubkey, err := p.ID.ExtractPublicKey(); err == nil {
		raw, _ := pubkey.Raw()
		if err := n.keydb.StoreKeys(
			context.Background(),
			map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult{
				{
					ServerName: gomatrixserverlib.ServerName(p.ID.String()),
					KeyID:      "ed25519:p2pdemo",
				}: {
					VerifyKey: gomatrixserverlib.VerifyKey{
						Key: gomatrixserverlib.Base64String(raw),
					},
					ValidUntilTS: math.MaxUint64 >> 1,
					ExpiredTS:    gomatrixserverlib.PublicKeyNotExpired,
				},
			},
		); err != nil {
			fmt.Println("Failed to store keys:", err)
		}
	}
	fmt.Println("Discovered", len(n.host.Peerstore().Peers())-1, "other libp2p peer(s):")
	for _, peer := range n.host.Peerstore().Peers() {
		if peer != n.host.ID() {
			fmt.Println("-", peer)
		}
	}
}
