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

package yggconn

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/neilalexander/utp"
	"github.com/sirupsen/logrus"

	ironwoodtypes "github.com/Arceliar/ironwood/types"
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
	utpSocket *utp.Socket
	incoming  chan net.Conn
}

func (n *Node) DialerContext(ctx context.Context, _, address string) (net.Conn, error) {
	tokens := strings.Split(address, ":")
	raw, err := hex.DecodeString(tokens[0])
	if err != nil {
		return nil, fmt.Errorf("hex.DecodeString: %w", err)
	}
	pk := make(ironwoodtypes.Addr, ed25519.PublicKeySize)
	copy(pk, raw[:])
	return n.utpSocket.DialAddrContext(ctx, pk)
}

func Setup(sk ed25519.PrivateKey, instanceName, storageDirectory, peerURI, listenURI string) (*Node, error) {
	n := &Node{
		log:      gologme.New(logrus.StandardLogger().Writer(), "", 0),
		incoming: make(chan net.Conn),
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
		if n.core, err = core.New(sk[:], n.log, options...); err != nil {
			panic(err)
		}
		n.core.SetLogger(n.log)

		if n.utpSocket, err = utp.NewSocketFromPacketConnNoClose(n.core); err != nil {
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
	go n.listenFromYgg()

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

func (n *Node) KnownNodes() []gomatrixserverlib.ServerName {
	nodemap := map[string]struct{}{}
	for _, peer := range n.core.GetPeers() {
		nodemap[hex.EncodeToString(peer.Key)] = struct{}{}
	}
	var nodes []gomatrixserverlib.ServerName
	for node := range nodemap {
		nodes = append(nodes, gomatrixserverlib.ServerName(node))
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
