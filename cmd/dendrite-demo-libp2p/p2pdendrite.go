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

	"errors"

	pstore "github.com/libp2p/go-libp2p-core/peerstore"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/matrix-org/dendrite/internal/setup"

	"github.com/libp2p/go-libp2p"
	circuit "github.com/libp2p/go-libp2p-circuit"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	routing "github.com/libp2p/go-libp2p-core/routing"

	host "github.com/libp2p/go-libp2p-core/host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/config"
)

// P2PDendrite is a Peer-to-Peer variant of BaseDendrite.
type P2PDendrite struct {
	Base setup.BaseDendrite

	// Store our libp2p object so that we can make outgoing connections from it
	// later
	LibP2P        host.Host
	LibP2PContext context.Context
	LibP2PCancel  context.CancelFunc
	LibP2PDHT     *dht.IpfsDHT
	LibP2PPubsub  *pubsub.PubSub
}

// NewP2PDendrite creates a new instance to be used by a component.
// The componentName is used for logging purposes, and should be a friendly name
// of the component running, e.g. SyncAPI.
func NewP2PDendrite(cfg *config.Dendrite, componentName string) *P2PDendrite {
	baseDendrite := setup.NewBaseDendrite(cfg, componentName, false)

	ctx, cancel := context.WithCancel(context.Background())

	privKey, err := crypto.UnmarshalEd25519PrivateKey(cfg.Global.PrivateKey[:])
	if err != nil {
		panic(err)
	}

	//defaultIP6ListenAddr, _ := multiaddr.NewMultiaddr("/ip6/::/tcp/0")
	var libp2pdht *dht.IpfsDHT
	libp2p, err := libp2p.New(ctx,
		libp2p.Identity(privKey),
		libp2p.DefaultListenAddrs,
		//libp2p.ListenAddrs(defaultIP6ListenAddr),
		libp2p.DefaultTransports,
		libp2p.Routing(func(h host.Host) (r routing.PeerRouting, err error) {
			libp2pdht, err = dht.New(ctx, h)
			if err != nil {
				return nil, err
			}
			libp2pdht.Validator = libP2PValidator{}
			r = libp2pdht
			return
		}),
		libp2p.EnableAutoRelay(),
		libp2p.EnableRelay(circuit.OptHop),
	)
	if err != nil {
		panic(err)
	}

	libp2ppubsub, err := pubsub.NewFloodSub(context.Background(), libp2p, []pubsub.Option{
		pubsub.WithMessageSigning(true),
	}...)
	if err != nil {
		panic(err)
	}

	fmt.Println("Our public key:", privKey.GetPublic())
	fmt.Println("Our node ID:", libp2p.ID())
	fmt.Println("Our addresses:", libp2p.Addrs())

	cfg.Global.ServerName = gomatrixserverlib.ServerName(libp2p.ID().String())

	return &P2PDendrite{
		Base:          *baseDendrite,
		LibP2P:        libp2p,
		LibP2PContext: ctx,
		LibP2PCancel:  cancel,
		LibP2PDHT:     libp2pdht,
		LibP2PPubsub:  libp2ppubsub,
	}
}

type libP2PValidator struct {
	KeyBook pstore.KeyBook
}

func (v libP2PValidator) Validate(key string, value []byte) error {
	ns, _, err := record.SplitKey(key)
	if err != nil || ns != "matrix" {
		return errors.New("not Matrix path")
	}
	return nil
}

func (v libP2PValidator) Select(k string, vals [][]byte) (int, error) {
	return 0, nil
}
