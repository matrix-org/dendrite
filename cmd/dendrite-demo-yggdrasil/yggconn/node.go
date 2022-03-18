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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/neilalexander/utp"

	ironwoodtypes "github.com/Arceliar/ironwood/types"
	yggdrasilconfig "github.com/yggdrasil-network/yggdrasil-go/src/config"
	yggdrasilcore "github.com/yggdrasil-network/yggdrasil-go/src/core"
	yggdrasildefaults "github.com/yggdrasil-network/yggdrasil-go/src/defaults"
	yggdrasilmulticast "github.com/yggdrasil-network/yggdrasil-go/src/multicast"

	gologme "github.com/gologme/log"
)

type Node struct {
	core      *yggdrasilcore.Core
	config    *yggdrasilconfig.NodeConfig
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

func Setup(instanceName, storageDirectory, peerURI string) (*Node, error) {
	n := &Node{
		core:      &yggdrasilcore.Core{},
		config:    yggdrasildefaults.GenerateConfig(),
		multicast: &yggdrasilmulticast.Multicast{},
		log:       gologme.New(os.Stdout, "YGG ", log.Flags()),
		incoming:  make(chan net.Conn),
	}

	yggfile := fmt.Sprintf("%s/%s-yggdrasil.conf", storageDirectory, instanceName)
	if _, err := os.Stat(yggfile); !os.IsNotExist(err) {
		yggconf, e := ioutil.ReadFile(yggfile)
		if e != nil {
			panic(err)
		}
		if err := json.Unmarshal([]byte(yggconf), &n.config); err != nil {
			panic(err)
		}
	}

	n.config.Peers = []string{}
	if peerURI != "" {
		n.config.Peers = append(n.config.Peers, peerURI)
	}
	n.config.AdminListen = "none"

	j, err := json.MarshalIndent(n.config, "", "  ")
	if err != nil {
		panic(err)
	}
	if e := ioutil.WriteFile(yggfile, j, 0600); e != nil {
		n.log.Printf("Couldn't write private key to file '%s': %s\n", yggfile, e)
	}

	n.log.EnableLevel("error")
	n.log.EnableLevel("warn")
	n.log.EnableLevel("info")
	if err = n.core.Start(n.config, n.log); err != nil {
		panic(err)
	}
	n.utpSocket, err = utp.NewSocketFromPacketConnNoClose(n.core)
	if err != nil {
		panic(err)
	}
	if err = n.multicast.Init(n.core, n.config, n.log, nil); err != nil {
		panic(err)
	}
	if err = n.multicast.Start(); err != nil {
		panic(err)
	}

	n.log.Println("Public key:", n.core.PublicKey())
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
	sk := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	sb, err := hex.DecodeString(n.config.PrivateKey)
	if err == nil {
		copy(sk, sb[:])
	} else {
		panic(err)
	}
	return sk
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
