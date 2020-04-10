package main

import (
	"context"
	"fmt"
	"math"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/gomatrixserverlib"
)

type mDNSListener struct {
	keydb keydb.Database
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
