// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package yggconn

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"net"
	"regexp"
	"strings"

	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"
	"github.com/yggdrasil-network/yggquic"

	"github.com/yggdrasil-network/yggdrasil-go/src/config"
	"github.com/yggdrasil-network/yggdrasil-go/src/core"
	yggdrasilcore "github.com/yggdrasil-network/yggdrasil-go/src/core"
	"github.com/yggdrasil-network/yggdrasil-go/src/multicast"
	yggdrasilmulticast "github.com/yggdrasil-network/yggdrasil-go/src/multicast"

	gologme "github.com/gologme/log"
)

type Node struct {
	core      *yggdrasilcore.Core
	multicast *yggdrasilmulticast.Multicast
	log       *gologme.Logger
	*yggquic.YggdrasilTransport
}

func (n *Node) DialerContext(ctx context.Context, _, address string) (net.Conn, error) {
	tokens := strings.Split(address, ":")
	return n.DialContext(ctx, "yggdrasil", tokens[0])
}

func Setup(sk ed25519.PrivateKey, instanceName, storageDirectory, peerURI, listenURI string) (*Node, error) {
	n := &Node{
		log: gologme.New(logrus.StandardLogger().Writer(), "", 0),
	}

	cfg := config.GenerateConfig()
	cfg.PrivateKey = config.KeyBytes(sk)
	if err := cfg.GenerateSelfSignedCertificate(); err != nil {
		panic(err)
	}

	n.log.EnableLevel("error")
	n.log.EnableLevel("warn")
	n.log.EnableLevel("info")

	{
		var err error
		options := []yggdrasilcore.SetupOption{}
		if listenURI != "" {
			options = append(options, yggdrasilcore.ListenAddress(listenURI))
		}
		if peerURI != "" {
			for _, uri := range strings.Split(peerURI, ",") {
				options = append(options, yggdrasilcore.Peer{
					URI: uri,
				})
			}
		}
		if n.core, err = core.New(cfg.Certificate, n.log, options...); err != nil {
			panic(err)
		}
		n.core.SetLogger(n.log)

		if n.YggdrasilTransport, err = yggquic.New(n.core, *cfg.Certificate, nil); err != nil {
			panic(err)
		}
	}

	// Setup the multicast module.
	{
		var err error
		options := []multicast.SetupOption{
			multicast.MulticastInterface{
				Regex:    regexp.MustCompile(".*"),
				Beacon:   true,
				Listen:   true,
				Port:     0,
				Priority: 0,
			},
		}
		if n.multicast, err = multicast.New(n.core, n.log, options...); err != nil {
			panic(err)
		}
	}

	n.log.Printf("Public key: %x", n.core.PublicKey())
	return n, nil
}

func (n *Node) Stop() {
	if err := n.multicast.Stop(); err != nil {
		n.log.Println("Error stopping multicast:", err)
	}
	n.core.Stop()
}

func (n *Node) DerivedServerName() string {
	return hex.EncodeToString(n.PublicKey())
}

func (n *Node) PrivateKey() ed25519.PrivateKey {
	return n.core.PrivateKey()
}

func (n *Node) PublicKey() ed25519.PublicKey {
	return n.core.PublicKey()
}

func (n *Node) PeerCount() int {
	return len(n.core.GetPeers())
}

func (n *Node) KnownNodes() []spec.ServerName {
	nodemap := map[string]struct{}{}
	for _, peer := range n.core.GetPeers() {
		nodemap[hex.EncodeToString(peer.Key)] = struct{}{}
	}
	var nodes []spec.ServerName
	for node := range nodemap {
		nodes = append(nodes, spec.ServerName(node))
	}
	return nodes
}

func (n *Node) SetMulticastEnabled(enabled bool) {
	// TODO: There's no dynamic reconfiguration in Yggdrasil v0.4
	// so we need a solution for this.
}

func (n *Node) DisconnectMulticastPeers() {
	// TODO: There's no dynamic reconfiguration in Yggdrasil v0.4
	// so we need a solution for this.
}

func (n *Node) DisconnectNonMulticastPeers() {
	// TODO: There's no dynamic reconfiguration in Yggdrasil v0.4
	// so we need a solution for this.
}

func (n *Node) SetStaticPeer(uri string) error {
	// TODO: There's no dynamic reconfiguration in Yggdrasil v0.4
	// so we need a solution for this.
	return nil
}
